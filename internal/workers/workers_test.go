package workers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAPI answers like Cloudflare, keyed by "METHOD /path", and records
// every request it gets.
type fakeAPI struct {
	replies  map[string]reply
	requests []recorded
}

type reply struct {
	status int
	result any
}

type recorded struct {
	route, token string
	header       http.Header
	body         []byte
}

func (f *fakeAPI) RoundTrip(req *http.Request) (*http.Response, error) {
	route := req.Method + " " + strings.TrimPrefix(req.URL.Path, "/client/v4")
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	f.requests = append(f.requests, recorded{route: route, token: strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "), header: req.Header, body: body})

	answer, ok := f.replies[route]
	if !ok {
		answer = reply{status: http.StatusNotFound}
	}
	if answer.status == 0 {
		answer.status = http.StatusOK
	}
	result, _ := json.Marshal(answer.result)
	envelope, _ := json.Marshal(map[string]any{"success": answer.status < 300, "errors": []any{}, "result": json.RawMessage(result)})
	return &http.Response{StatusCode: answer.status, Status: http.StatusText(answer.status), Body: io.NopCloser(strings.NewReader(string(envelope))), Header: http.Header{}}, nil
}

func newClient(replies map[string]reply) (*Client, *fakeAPI) {
	api := &fakeAPI{replies: replies}
	return &Client{Token: "api-token", HTTP: &http.Client{Transport: api}}, api
}

func (f *fakeAPI) find(t *testing.T, route string) recorded {
	t.Helper()
	for _, request := range f.requests {
		if request.route == route {
			return request
		}
	}
	t.Fatalf("no request to %s; got %v", route, f.routes())
	return recorded{}
}

func (f *fakeAPI) routes() []string {
	var routes []string
	for _, request := range f.requests {
		routes = append(routes, request.route)
	}
	return routes
}

func formParts(t *testing.T, request recorded) map[string]*multipart.Part {
	t.Helper()
	_, params, err := mime.ParseMediaType(request.header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(strings.NewReader(string(request.body)), params["boundary"])
	parts := map[string]*multipart.Part{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return parts
		}
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(part)
		part.Header.Set("X-Content", string(content))
		parts[part.FormName()] = part
	}
}

func writeSite(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDeployUploadsWhatCloudflareAsksFor(t *testing.T) {
	dir := writeSite(t, map[string]string{"index.html": "<h1>Bakery</h1>", "about/index.html": "<h1>About</h1>", "404.html": "gone"})
	entries, _, err := manifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries["/about/index.html"].Size != int64(len("<h1>About</h1>")) || len(entries["/index.html"].Hash) != 32 {
		t.Fatalf("manifest: %+v", entries)
	}
	changed := entries["/about/index.html"].Hash

	const worker = "/accounts/acc/workers/scripts/bakery"
	client, api := newClient(map[string]reply{
		"POST " + worker + "/assets-upload-session": {result: map[string]any{"jwt": "upload-token", "buckets": [][]string{{changed}}}},
		"POST /accounts/acc/workers/assets/upload":  {status: http.StatusCreated, result: map[string]string{"jwt": "completion-token"}},
		"PUT " + worker:                 {},
		"POST " + worker + "/subdomain": {},
	})
	if err := client.Deploy(context.Background(), "acc", "bakery", dir); err != nil {
		t.Fatal(err)
	}

	upload := api.find(t, "POST /accounts/acc/workers/assets/upload")
	if upload.token != "upload-token" {
		t.Errorf("files were uploaded with %q, want the upload token", upload.token)
	}
	parts := formParts(t, upload)
	if len(parts) != 1 || parts[changed] == nil {
		t.Fatalf("uploaded %v, want only the changed file", parts)
	}
	if content, _ := base64.StdEncoding.DecodeString(parts[changed].Header.Get("X-Content")); string(content) != "<h1>About</h1>" {
		t.Errorf("uploaded %q", content)
	}
	if contentType := parts[changed].Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("Content-Type = %q", contentType)
	}

	script := formParts(t, api.find(t, "PUT "+worker))
	var metadata struct {
		MainModule string `json:"main_module"`
		Assets     struct {
			JWT    string            `json:"jwt"`
			Config map[string]string `json:"config"`
		} `json:"assets"`
	}
	if err := json.Unmarshal([]byte(script["metadata"].Header.Get("X-Content")), &metadata); err != nil {
		t.Fatal(err)
	}
	if len(script) != 1 || metadata.MainModule != "" || metadata.Assets.JWT != "completion-token" || metadata.Assets.Config["not_found_handling"] != "404-page" {
		t.Errorf("the Worker should only serve the files: %+v, parts %v", metadata, script)
	}
	if subdomain := string(api.find(t, "POST "+worker+"/subdomain").body); !strings.Contains(subdomain, `"enabled":true`) {
		t.Errorf("workers.dev address not turned on: %s", subdomain)
	}
}

func TestDeploySkipsUploadWhenNothingChanged(t *testing.T) {
	const worker = "/accounts/acc/workers/scripts/bakery"
	client, api := newClient(map[string]reply{
		"POST " + worker + "/assets-upload-session": {result: map[string]any{"jwt": "completion-token", "buckets": [][]string{}}},
		"PUT " + worker:                 {},
		"POST " + worker + "/subdomain": {},
	})
	if err := client.Deploy(context.Background(), "acc", "bakery", writeSite(t, map[string]string{"index.html": "hi"})); err != nil {
		t.Fatal(err)
	}
	if routes := strings.Join(api.routes(), ","); strings.Contains(routes, "assets/upload") {
		t.Errorf("uploaded files Cloudflare already had: %s", routes)
	}
	if script := formParts(t, api.find(t, "PUT "+worker)); !strings.Contains(script["metadata"].Header.Get("X-Content"), "completion-token") {
		t.Errorf("metadata: %s", script["metadata"].Header.Get("X-Content"))
	}
}

func TestRejectedTokenIsErrAuth(t *testing.T) {
	client, _ := newClient(map[string]reply{"GET /accounts": {status: http.StatusForbidden}})
	if _, err := client.Accounts(context.Background()); !errors.Is(err, ErrAuth) {
		t.Errorf("got %v, want ErrAuth", err)
	}
}

func TestSubdomain(t *testing.T) {
	const endpoint = "/accounts/acc/workers/subdomain"
	client, api := newClient(map[string]reply{"GET " + endpoint: {result: map[string]string{"subdomain": "hannah"}}})
	if subdomain, err := client.Subdomain(context.Background(), "acc", "bakery"); err != nil || subdomain != "hannah" {
		t.Errorf("existing subdomain: %q %v", subdomain, err)
	}
	if len(api.requests) != 1 {
		t.Errorf("registered a subdomain when the account has one: %v", api.routes())
	}

	client, api = newClient(map[string]reply{"PUT " + endpoint: {result: map[string]string{"subdomain": "bakery"}}})
	if subdomain, err := client.Subdomain(context.Background(), "acc", "bakery"); err != nil || subdomain != "bakery" {
		t.Errorf("new account: %q %v", subdomain, err)
	}
	if !strings.Contains(string(api.find(t, "PUT "+endpoint).body), `"subdomain":"bakery"`) {
		t.Errorf("registered %s", api.find(t, "PUT "+endpoint).body)
	}
}

func TestWorkerExists(t *testing.T) {
	client, _ := newClient(map[string]reply{"GET /accounts/acc/workers/scripts/taken/settings": {}})
	for name, want := range map[string]bool{"taken": true, "free": false} {
		if exists, err := client.WorkerExists(context.Background(), "acc", name); err != nil || exists != want {
			t.Errorf("%s: %v %v", name, exists, err)
		}
	}
}
