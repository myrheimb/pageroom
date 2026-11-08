package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "Pageroom/") {
			t.Errorf("missing Pageroom User-Agent, got %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestCheck(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		current string
		want    string
	}{
		{"newer", 200, `{"tag_name":"v0.2.0"}`, "0.1.0", "0.2.0"},
		{"newer without prefix", 200, `{"tag_name":"0.1.1"}`, "0.1.0", "0.1.1"},
		{"numeric not textual", 200, `{"tag_name":"v0.10.0"}`, "0.9.0", "0.10.0"},
		{"equal", 200, `{"tag_name":"v0.1.0"}`, "0.1.0", ""},
		{"older", 200, `{"tag_name":"v0.0.9"}`, "0.1.0", ""},
		{"bad tag", 200, `{"tag_name":"nightly"}`, "0.1.0", ""},
		{"prerelease tag", 200, `{"tag_name":"v0.2.0-rc1"}`, "0.1.0", ""},
		{"not json", 200, `<html>`, "0.1.0", ""},
		{"rate limited", 403, `{"tag_name":"v0.2.0"}`, "0.1.0", ""},
		{"not found", 404, `{}`, "0.1.0", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := serve(t, tc.status, tc.body)
			got := Check(context.Background(), server.Client(), server.URL, tc.current)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil || got.Version != tc.want {
				t.Fatalf("want %s, got %+v", tc.want, got)
			}
			if !strings.HasSuffix(got.URL, "/releases/tag/v"+tc.want) {
				t.Fatalf("unexpected URL %s", got.URL)
			}
		})
	}
}

func TestCheckOffline(t *testing.T) {
	server := serve(t, 200, `{}`)
	url := server.URL
	server.Close()
	if got := Check(context.Background(), http.DefaultClient, url, "0.1.0"); got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}
