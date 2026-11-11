package secrets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestStoreRoundTripAndRedaction(t *testing.T) {
	s := &Store{Backend: &memoryBackend{}, Redactor: &Redactor{}}
	if _, err := s.Get("https://github.com/a/b.git"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	c := Credential{Kind: "token", Username: "me", Secret: "supersecretvalue123"}
	if err := s.Save("https://github.com/a/b.git", c); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("https://GitHub.com/other/repo")
	if err != nil || got.Secret != c.Secret {
		t.Fatalf("host-level lookup failed: %v %v", got, err)
	}
	msg := s.Redactor.String("push failed for supersecretvalue123 at https://me:pw12345@host/x")
	if strings.Contains(msg, "supersecret") || strings.Contains(msg, "pw12345") {
		t.Fatalf("not redacted: %s", msg)
	}
	if e := s.Redactor.Error(fmt.Errorf("token ghp_%s rejected", strings.Repeat("a", 36))); strings.Contains(e.Error(), "ghp_") {
		t.Fatalf("token shape not redacted: %v", e)
	}
}

func TestHostKey(t *testing.T) {
	cases := map[string]string{
		"https://github.com/a/b.git":  "git:https://github.com",
		"git@github.com:a/b.git":      "git:ssh://github.com",
		"https://user@gitlab.com/x/y": "git:https://gitlab.com",
	}
	for in, want := range cases {
		if got := HostKey(in); got != want {
			t.Errorf("HostKey(%q)=%q want %q", in, got, want)
		}
	}
}

func TestStripURLCredentials(t *testing.T) {
	clean, user, pass, ok := StripURLCredentials("https://ghp_abc123@github.com/a/b.git")
	if !ok || clean != "https://github.com/a/b.git" || pass != "ghp_abc123" || user != "x-access-token" {
		t.Fatalf("got %q %q %q %v", clean, user, pass, ok)
	}
	if _, _, _, ok := StripURLCredentials("https://github.com/a/b.git"); ok {
		t.Fatal("clean URL reported as changed")
	}
}

func TestScanFile(t *testing.T) {
	if f := ScanFile(".env", []byte("A=1")); len(f) != 1 {
		t.Fatalf(".env not flagged: %v", f)
	}
	if f := ScanFile(".env.example", []byte("A=1")); len(f) != 0 {
		t.Fatalf(".env.example flagged: %v", f)
	}
	body := "---\ntitle: Hi\n---\nmy key is AKIAABCDEFGHIJKLMNOP oops\n"
	f := ScanFile("content/post.md", []byte(body))
	if len(f) != 1 || f[0].Line != 4 {
		t.Fatalf("AWS key not found on line 4: %v", f)
	}
	if f := ScanFile("hugo.toml", []byte("apiKey = \"YOUR_API_KEY_HERE_123\"")); len(f) != 0 {
		t.Fatalf("placeholder flagged: %v", f)
	}
	leaky := "# see example.com\npassword = hunter2hunter2hunter2 # your_team knows\n"
	if f := ScanFile("hugo.toml", []byte(leaky)); len(f) != 1 || f[0].Line != 2 {
		t.Fatalf("real password on a line mentioning example/your_ not flagged: %v", f)
	}
	if f := ScanFile("img.png", []byte{0x89, 'P', 'N', 'G', 0, 0, 'A', 'K', 'I', 'A'}); len(f) != 0 {
		t.Fatal("binary scanned")
	}
}

func TestDeviceFlow(t *testing.T) {
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			_, _ = fmt.Fprint(w, `{"device_code":"dc","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":900,"interval":0}`)
		case "/login/oauth/access_token":
			if atomic.AddInt32(&polls, 1) < 2 {
				_, _ = fmt.Fprint(w, `{"error":"authorization_pending"}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"access_token":"ghu_token","refresh_token":"ghr_refresh","expires_in":28800}`)
		}
	}))
	defer srv.Close()
	g := &GitHubAuth{ClientID: "cid", BaseURL: srv.URL}
	dc, err := g.Start(context.Background())
	if err != nil || dc.UserCode != "ABCD-1234" {
		t.Fatalf("start: %v %v", dc, err)
	}
	dc.Interval = 0
	c, err := g.Wait(context.Background(), dc)
	if err != nil || c.Secret != "ghu_token" || c.RefreshToken != "ghr_refresh" || c.Expires.IsZero() {
		t.Fatalf("wait: %+v %v", c, err)
	}
}

// memoryBackend is an in-memory Backend.
type memoryBackend struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memoryBackend) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (s *memoryBackend) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = value
	return nil
}
func (s *memoryBackend) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}
