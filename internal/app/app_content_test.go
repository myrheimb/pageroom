package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/starter"
)

func shortcodeFiles(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "layouts", "shortcodes"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names
}

func newBlockSite(t *testing.T) (*App, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(root, "Blocks", "0.166.0"); err != nil {
		t.Fatal(err)
	}
	s, err := site.Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &App{site: s}, root
}

func TestAddPageBlockWritesTemplates(t *testing.T) {
	cases := map[string][]string{
		"button": {"pr-button.html"},
		"cards":  {"pr-card.html", "pr-cards.html"},
		"hero":   {"pr-hero.html"},
	}
	for kind, want := range cases {
		t.Run(kind, func(t *testing.T) {
			a, root := newBlockSite(t)
			if err := a.AddPageBlock(kind); err != nil {
				t.Fatal(err)
			}
			if got := shortcodeFiles(t, root); !slices.Equal(got, want) {
				t.Fatalf("templates = %v, want %v", got, want)
			}
		})
	}
}

func TestAddPageBlockRejectsUnknownKind(t *testing.T) {
	a, root := newBlockSite(t)
	before := shortcodeFiles(t, root)
	if err := a.AddPageBlock("carousel"); err == nil {
		t.Fatal("expected an error for an unknown kind")
	}
	if got := shortcodeFiles(t, root); !slices.Equal(got, before) {
		t.Fatalf("files changed: %v -> %v", before, got)
	}
}
