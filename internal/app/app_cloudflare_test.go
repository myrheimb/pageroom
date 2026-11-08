package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrheimb/pageroom/internal/gitsync"
	"github.com/myrheimb/pageroom/internal/hugo"
	"github.com/myrheimb/pageroom/internal/secrets"
	"github.com/myrheimb/pageroom/internal/settings"
	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/starter"
)

// fakeCloudflare answers "METHOD /path" with a result, or with an error
// when the result is an HTTP status, or 404 for routes it doesn't know. It
// records the routes it was asked for.
type fakeCloudflare struct {
	results map[string]any
	routes  []string
}

func (f *fakeCloudflare) RoundTrip(req *http.Request) (*http.Response, error) {
	route := req.Method + " " + strings.TrimPrefix(req.URL.Path, "/client/v4")
	f.routes = append(f.routes, route)
	result, ok := f.results[route]
	status := http.StatusOK
	if code, isStatus := result.(int); isStatus {
		status = code
	} else if !ok {
		status = http.StatusNotFound
	}
	body, _ := json.Marshal(map[string]any{"success": status < 300, "result": result})
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
}

// newLocalSite is a starter site that is only on this computer.
func newLocalSite(t *testing.T, results map[string]any) (*App, *fakeCloudflare) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(root, "Bakery", "0.166.0"); err != nil {
		t.Fatal(err)
	}
	repo, err := gitsync.Init(root, gitsync.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CommitAll("Create Bakery"); err != nil {
		t.Fatal(err)
	}
	s, err := site.Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := &fakeCloudflare{results: results}
	a := &App{
		ctx:            context.Background(),
		site:           s,
		repo:           repo,
		creds:          &secrets.Store{Backend: &memoryBackend{}, Redactor: redactor},
		cloudflareHTTP: &http.Client{Transport: api},
	}
	return a, api
}

var oneAccount = map[string]any{
	"GET /accounts":                               []map[string]string{{"id": "acc", "name": "Hannah"}},
	"GET /accounts/acc/workers/subdomain":         map[string]string{"subdomain": "hannah"},
	"DELETE /accounts/acc/workers/scripts/bakery": nil,
}

func connect(t *testing.T, a *App) {
	t.Helper()
	if accounts, err := a.ConnectCloudflare(" token ", ""); err != nil || accounts != nil {
		t.Fatalf("connect: %v %v", accounts, err)
	}
}

func TestWorkerName(t *testing.T) {
	for _, test := range []struct {
		names []string
		want  string
	}{
		{[]string{"Bakery"}, "bakery"},
		{[]string{"Hannah's Bakery & Café!"}, "hannahs-bakery-cafe"},
		{[]string{"パン屋", "pan-ya"}, "pan-ya"},
		{[]string{"パン屋 Hannah", "pan-ya"}, "hannah"},
		{[]string{"パン屋", "パン屋"}, "site"},
		{[]string{strings.Repeat("long-", 20)}, strings.TrimSuffix(strings.Repeat("long-", 10), "-")},
	} {
		if got := workerName(test.names...); got != test.want {
			t.Errorf("workerName(%q) = %q, want %q", test.names, got, test.want)
		}
	}
}

func TestConnectCloudflare(t *testing.T) {
	results := map[string]any{"GET /accounts/acc/workers/scripts/bakery/settings": map[string]any{}}
	for route, result := range oneAccount {
		results[route] = result
	}
	a, _ := newLocalSite(t, results)
	connect(t, a)

	want := DirectUpload{Account: "acc", Worker: "bakery-2", Address: "https://bakery-2.hannah.workers.dev/"}
	if direct := a.gitState().Direct; direct == nil || *direct != want {
		t.Fatalf("got %+v, want %+v, without taking over the existing Worker", direct, want)
	}
	if baseURL := a.site.Settings().BaseURL; baseURL != want.Address {
		t.Errorf("baseURL = %q", baseURL)
	}
	if credential, err := a.creds.Get(cloudflareAPI); err != nil || credential.Secret != "token" {
		t.Errorf("token not saved: %+v %v", credential, err)
	}
	if _, err := a.ConnectRemote("https://github.com/hannah/bakery"); err == nil {
		t.Error("connected a repository while Pageroom uploads the site itself")
	}
}

func TestConnectCloudflareAsksWhichAccount(t *testing.T) {
	a, _ := newLocalSite(t, map[string]any{"GET /accounts": []map[string]string{{"id": "one"}, {"id": "two"}}})
	accounts, err := a.ConnectCloudflare("token", "")
	if err != nil || len(accounts) != 2 {
		t.Fatalf("got %v %v, want both accounts", accounts, err)
	}
	if a.gitState().Direct != nil {
		t.Error("connected before the account was chosen")
	}
}

func TestConnectCloudflareRejectsBadToken(t *testing.T) {
	a, _ := newLocalSite(t, map[string]any{"GET /accounts": http.StatusForbidden})
	_, err := a.ConnectCloudflare("token", "")
	if appError, ok := err.(*AppError); !ok || appError.Code != "cloudflare-token" {
		t.Errorf("got %v", err)
	}
}

func TestPublishWithoutChangesIsUpToDate(t *testing.T) {
	a, api := newLocalSite(t, oneAccount)
	connect(t, a)
	direct := a.gitState().Direct
	direct.Published = true
	if err := saveDirectUpload(a.repo, direct); err != nil {
		t.Fatal(err)
	}
	if err := a.updateLetterIcon(a.site); err != nil {
		t.Fatal(err)
	}
	if err := a.repo.CommitAll("Set the address"); err != nil {
		t.Fatal(err)
	}

	calls := len(api.routes)
	out, err := a.Publish("")
	if err != nil || out.State != "up-to-date" || len(api.routes) != calls {
		t.Errorf("got %+v %v, Cloudflare calls %v", out, err, api.routes[calls:])
	}
}

func TestStopCloudflare(t *testing.T) {
	a, api := newLocalSite(t, oneAccount)
	connect(t, a)
	if err := a.StopCloudflare(); err != nil {
		t.Fatal(err)
	}
	if routes := strings.Join(api.routes, ","); !strings.Contains(routes, "DELETE /accounts/acc/workers/scripts/bakery") {
		t.Errorf("the Worker wasn't deleted: %s", routes)
	}
	if direct := a.gitState().Direct; direct != nil {
		t.Errorf("still connected: %+v", direct)
	}
	if baseURL := a.site.Settings().BaseURL; baseURL != "" {
		t.Errorf("baseURL = %q", baseURL)
	}
}

// Runs only where the starter site's Hugo version is installed.
func TestPublishUploadsThenSavesChanges(t *testing.T) {
	const worker = "/accounts/acc/workers/scripts/bakery"
	results := map[string]any{
		"POST " + worker + "/assets-upload-session": map[string]any{"jwt": "completion-token", "buckets": [][]string{}},
		"PUT " + worker:                 http.StatusInternalServerError,
		"POST " + worker + "/subdomain": nil,
	}
	for route, result := range oneAccount {
		results[route] = result
	}
	a, api := newLocalSite(t, results)
	a.hugo = &hugo.Manager{CacheDir: t.TempDir()}
	a.dirs = settings.Dirs{Cache: t.TempDir()}
	if _, _, err := a.hugoBin(a.site); err != nil {
		t.Skip("Hugo 0.166.0 not installed")
	}
	connect(t, a)
	if err := os.WriteFile(filepath.Join(a.site.Root, "content", "prices.md"), []byte("+++\ntitle = 'Prices'\n+++\nWe bake.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := a.Publish(""); err == nil {
		t.Fatal("publishing should fail when Cloudflare does")
	}
	if changes, _ := a.GetChanges(); len(changes) == 0 || a.gitState().Direct.Published {
		t.Fatalf("a failed upload must leave the changes unpublished: %v", changes)
	}

	api.results["PUT "+worker] = nil
	out, err := a.Publish("")
	if err != nil || out.State != "published" {
		t.Fatalf("publish: %+v %v", out, err)
	}
	if changes, _ := a.GetChanges(); len(changes) != 0 || !a.gitState().Direct.Published {
		t.Errorf("still unpublished: %v", changes)
	}
	if built, err := os.ReadFile(filepath.Join(a.workDir(a.site), "live", "prices", "index.html")); err != nil || !strings.Contains(string(built), "We bake.") {
		t.Errorf("the uploaded site is missing the new page: %v", err)
	}
}

type memoryBackend struct{ values map[string]string }

func (backend *memoryBackend) Get(key string) (string, error) {
	value, ok := backend.values[key]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return value, nil
}

func (backend *memoryBackend) Set(key, value string) error {
	if backend.values == nil {
		backend.values = map[string]string{}
	}
	backend.values[key] = value
	return nil
}

func (backend *memoryBackend) Delete(key string) error {
	delete(backend.values, key)
	return nil
}
