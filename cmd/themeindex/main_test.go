package main

import "testing"

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
