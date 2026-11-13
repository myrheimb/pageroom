package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestHostURLs(t *testing.T) {
	const commit = "abc123"
	for _, test := range []struct {
		module, repo, rest, archive, raw string
	}{
		{
			"github.com/owner/paper", "github.com/owner/paper", "",
			"https://github.com/owner/paper/archive/abc123.tar.gz",
			"https://raw.githubusercontent.com/owner/paper/abc123/images/tn.png",
		},
		{
			"github.com/owner/stack/v3", "github.com/owner/stack", "v3",
			"https://github.com/owner/stack/archive/abc123.tar.gz",
			"https://raw.githubusercontent.com/owner/stack/abc123/images/tn.png",
		},
		{
			"gitlab.com/owner/gruv", "gitlab.com/owner/gruv", "",
			"https://gitlab.com/owner/gruv/-/archive/abc123/gruv-abc123.tar.gz",
			"https://gitlab.com/owner/gruv/-/raw/abc123/images/tn.png",
		},
		{
			"codeberg.org/owner/liftoff/modules/theme", "codeberg.org/owner/liftoff", "modules/theme",
			"https://codeberg.org/owner/liftoff/archive/abc123.tar.gz",
			"https://codeberg.org/owner/liftoff/raw/commit/abc123/images/tn.png",
		},
	} {
		repo, rest := splitModule(test.module)
		archive, err := archiveURL(repo, commit)
		raw := rawURL(repo, commit, "images/tn.png")
		if repo != test.repo || rest != test.rest || archive != test.archive || raw != test.raw || err != nil {
			t.Errorf("%s: got %s %q %s %s %v", test.module, repo, rest, archive, raw, err)
		}
	}

	if _, err := archiveURL("go.example.com/theme", commit); err == nil {
		t.Error("expected unsupported hosts to be rejected")
	}
}

func TestAuthorName(t *testing.T) {
	for _, test := range []struct{ toml, want string }{
		{"[author]\nname = ' Ada '", "Ada"},
		{"author = 'Ada'", "Ada"},
		{"name = 'Theme'", ""},
	} {
		var meta themeMeta
		if err := toml.Unmarshal([]byte(test.toml), &meta); err != nil {
			t.Fatalf("%q: %v", test.toml, err)
		}
		if got := authorName(meta.Author); got != test.want {
			t.Errorf("%q: got %q, want %q", test.toml, got, test.want)
		}
	}
}

func TestFindImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"tn.jpg", "screenshot.png", "screenshot.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, "images", file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for name, want := range map[string]string{"tn": "images/tn.jpg", "screenshot": "images/screenshot.png", "banner": ""} {
		if got := findImage(dir, name); got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestBuildErrorKeepsTheCause(t *testing.T) {
	err := errors.New("ERROR render of \"" + strings.Repeat("/long/path", 100) + "\" failed:\n  can't evaluate field Author")
	got := buildError(err)
	if !strings.HasSuffix(got, "failed: can't evaluate field Author") || len([]rune(got)) != 401 {
		t.Errorf("got %q", got)
	}
}
