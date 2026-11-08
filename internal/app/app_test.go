package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/myrheimb/pageroom/internal/gitsync"
	"github.com/myrheimb/pageroom/internal/hugo"
	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/starter"
)

func TestSummarizeChanges(t *testing.T) {
	cases := []struct {
		in   []ChangeView
		want string
	}{
		{nil, "Update site"},
		{[]ChangeView{{Type: "page", Label: "About"}}, "Update “About”"},
		{[]ChangeView{{Type: "page", Label: "A"}, {Type: "image"}, {Type: "image"}}, "Update “A”, 2 images"},
		{[]ChangeView{{Type: "appearance"}, {Type: "appearance"}}, "Update appearance"},
		{[]ChangeView{{Type: "theme"}, {Type: "settings"}}, "Update appearance, site settings"},
		{[]ChangeView{{Type: "settings"}, {Type: "file"}}, "Update site settings, 1 other file"},
	}
	for _, c := range cases {
		if got := SummarizeChanges(c.in); got != c.want {
			t.Errorf("SummarizeChanges(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGitHubPagesAddress(t *testing.T) {
	for _, test := range []struct{ remote, address string }{
		{"https://github.com/Hannah/bakery", "https://hannah.github.io/bakery/"},
		{"https://github.com/hannah/bakery.git", "https://hannah.github.io/bakery/"},
		{"git@github.com:hannah/bakery.git", "https://hannah.github.io/bakery/"},
		{"ssh://git@github.com/hannah/bakery", "https://hannah.github.io/bakery/"},
		{"https://github.com/Hannah/hannah.github.io", "https://hannah.github.io/"},
		{"https://gitlab.com/hannah/bakery", ""},
		{"https://github.com/hannah", ""},
	} {
		address := ""
		if pages := githubPages(test.remote, t.TempDir()); pages != nil {
			address = pages.Address
		}
		if address != test.address {
			t.Errorf("%s: got %q, want %q", test.remote, address, test.address)
		}
	}
}

func TestSetUpGitHubPages(t *testing.T) {
	a, root := newHostedSite(t)
	s := a.site

	pages, err := a.SetUpGitHubPages()
	if err != nil || !pages.SetUp || pages.Settings != "https://github.com/hannah/bakery/settings/pages" {
		t.Fatalf("set up: %+v %v", pages, err)
	}
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "pages.yml"))
	if err != nil || !strings.Contains(string(workflow), `branches: ["main"]`) || !strings.Contains(string(workflow), "HUGO_VERSION: 0.166.0") {
		t.Fatalf("workflow: %v\n%s", err, workflow)
	}
	if version := hugo.SiteVersion(root, ""); version != "0.166.0" {
		t.Errorf("the workflow's Hugo version reads as %q", version)
	}
	if baseURL := s.Settings().BaseURL; baseURL != "https://hannah.github.io/bakery/" {
		t.Errorf("baseURL = %q", baseURL)
	}
	if again, err := a.SetUpGitHubPages(); err != nil || !again.SetUp {
		t.Errorf("setting up twice: %+v %v", again, err)
	}
}

func TestCloudflareWorkerName(t *testing.T) {
	for _, test := range []struct{ remote, name string }{
		{"https://github.com/hannah/bakery", "bakery"},
		{"git@github.com:hannah/My.Bakery.git", "my-bakery"},
		{"https://gitlab.com/hannah/bakery-site/", "bakery-site"},
		{"https://bitbucket.org/hannah/bakery", ""},
	} {
		name := ""
		if setup := cloudflare(test.remote, t.TempDir()); setup != nil {
			name = setup.Name
		}
		if name != test.name {
			t.Errorf("%s: got %q, want %q", test.remote, name, test.name)
		}
	}
}

func TestSetUpCloudflare(t *testing.T) {
	a, root := newHostedSite(t)

	setup, err := a.SetUpCloudflare()
	if err != nil || !setup.SetUp || setup.Name != "bakery" {
		t.Fatalf("set up: %+v %v", setup, err)
	}
	config, _ := os.ReadFile(filepath.Join(root, "wrangler.jsonc"))
	build, _ := os.ReadFile(filepath.Join(root, "build.sh"))
	if !strings.Contains(string(config), `"name": "bakery"`) || strings.Contains(string(config), "{{") || !strings.Contains(string(build), `HUGO_VERSION="0.166.0"`) {
		t.Fatalf("wrangler.jsonc:\n%s\nbuild.sh:\n%s", config, build)
	}
	if again, err := a.SetUpCloudflare(); err != nil || !again.SetUp {
		t.Errorf("setting up twice: %+v %v", again, err)
	}
}

// newHostedSite is a starter site whose repository is on GitHub.
func newHostedSite(t *testing.T) (*App, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(root, "Bakery", "0.166.0"); err != nil {
		t.Fatal(err)
	}
	repo, err := gitsync.Init(root, gitsync.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetRemote("https://github.com/hannah/bakery"); err != nil {
		t.Fatal(err)
	}
	s, err := site.Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &App{site: s, repo: repo}
	return a, root
}

func TestRemoveHosting(t *testing.T) {
	for _, test := range []struct {
		host, address, keptAddress string
		files                      []string
	}{
		{"github-pages", "", "", []string{".github"}},
		{"cloudflare", "https://bakery.hannah.workers.dev/", "", []string{"wrangler.jsonc", "build.sh"}},
		{"cloudflare", "https://bakery.no/", "https://bakery.no/", []string{"wrangler.jsonc", "build.sh"}},
	} {
		a, root := newHostedSite(t)
		set := map[string]func() error{
			"github-pages": func() error { _, err := a.SetUpGitHubPages(); return err },
			"cloudflare":   func() error { _, err := a.SetUpCloudflare(); return err },
		}[test.host]
		if err := set(); err != nil {
			t.Fatal(err)
		}
		if test.address != "" {
			settings := a.site.Settings()
			settings.BaseURL = test.address
			if err := a.site.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
		}

		if err := a.RemoveHosting(test.host); err != nil {
			t.Fatalf("%s: %v", test.host, err)
		}
		for _, file := range test.files {
			if _, err := os.Stat(filepath.Join(root, file)); err == nil {
				t.Errorf("%s: %s is still there", test.host, file)
			}
		}
		if baseURL := a.site.Settings().BaseURL; baseURL != test.keptAddress {
			t.Errorf("%s: baseURL = %q, want %q", test.host, baseURL, test.keptAddress)
		}
		remote := a.repo.RemoteURL()
		if pages, worker := githubPages(remote, root), cloudflare(remote, root); pages.SetUp || worker.SetUp {
			t.Errorf("%s: still set up: %+v %+v", test.host, pages, worker)
		}
	}
}

func TestRemoveHostingLeavesDevelopersFilesAlone(t *testing.T) {
	a, root := newHostedSite(t)
	if _, err := a.SetUpCloudflare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "build.sh"), []byte("#!/bin/sh\nhugo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveHosting("cloudflare"); err == nil {
		t.Fatal("expected an error for a developer's build.sh")
	}
	if _, err := os.Stat(filepath.Join(root, "wrangler.jsonc")); err != nil {
		t.Error("nothing should be removed when a file isn't Pageroom's")
	}
}

func TestDisconnectRemoteRemovesHostingSetup(t *testing.T) {
	a, root := newHostedSite(t)
	if _, err := a.SetUpCloudflare(); err != nil {
		t.Fatal(err)
	}
	if err := a.DisconnectRemote(); err != nil {
		t.Fatal(err)
	}
	if a.repo.RemoteURL() != "" {
		t.Errorf("still connected to %s", a.repo.RemoteURL())
	}
	if _, err := os.Stat(filepath.Join(root, "wrangler.jsonc")); err == nil {
		t.Error("the Cloudflare setup belongs to the old repository and should be gone")
	}
}

func TestShellCharacterEncoderSatisfiesWails(t *testing.T) {
	// Copied from Wails' ValidateAndSanitizeURL, which is internal.
	wailsRejects := regexp.MustCompile(`[;\|` + "`" + `$\\<>*{}\[\]()~! \t\n\r]`)

	for _, link := range []string{
		"https://dash.cloudflare.com/profile/api-tokens?permissionGroupKeys=%5B%5D&accountId=*&zoneId=all",
		"https://example.com/a (b)/~c?q=x!y;z$&r=[1]{2}|<3>\\`",
	} {
		encoded := shellCharacterEncoder.Replace(link)
		if wailsRejects.MatchString(encoded) {
			t.Errorf("Wails would refuse %q", encoded)
		}

		decodedLink, _ := url.PathUnescape(link)
		decodedEncoded, _ := url.PathUnescape(encoded)
		if decodedEncoded != decodedLink {
			t.Errorf("%q decodes to %q, want %q", encoded, decodedEncoded, decodedLink)
		}
	}
}

func TestSiteFilesServesFontsAndImagesOnly(t *testing.T) {
	a, root := newHostedSite(t)
	for name, content := range map[string]string{"static/fonts/a.woff2": "font", "static/notes.txt": "text"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]struct {
		status      int
		contentType string
	}{
		"/site-file/static/fonts/a.woff2": {http.StatusOK, "font/woff2"},
		"/site-file/static/notes.txt":     {http.StatusNotFound, ""},
	}
	for path, want := range cases {
		recorder := httptest.NewRecorder()
		SiteFiles(a).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != want.status {
			t.Errorf("%s: status %d, want %d", path, recorder.Code, want.status)
		}
		if want.contentType != "" && recorder.Header().Get("Content-Type") != want.contentType {
			t.Errorf("%s: content type %q, want %q", path, recorder.Header().Get("Content-Type"), want.contentType)
		}
	}
}
