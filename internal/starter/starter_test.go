package starter

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/myrheimb/pageroom/internal/site"
)

func TestCreate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	if err := Create(dir, `Bob's "Great" Site`, "0.168.0"); err != nil {
		t.Fatal(err)
	}
	s, err := site.Open(dir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Settings().Title; got != `Bob's "Great" Site` {
		t.Fatalf("title %q", got)
	}
	for _, f := range []string{".gitignore", ".pageroom.yaml", "themes/pageroom-paper/theme.toml", "themes/pageroom-soft/theme.toml", "themes/pageroom-studio/theme.toml", "themes/pageroom-tinted/theme.toml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	prefs, _ := os.ReadFile(filepath.Join(dir, ".pageroom.yaml"))
	if !strings.Contains(string(prefs), `"0.168.0"`) {
		t.Error("hugo version not pinned")
	}
	if err := Create(dir, "x", "0.168.0"); err == nil {
		t.Fatal("overwrote non-empty folder")
	}
}

// Hugo only uses a translation file whose name matches the site's locale
// exactly, so "ja" and "ja-JP" sites each need their own copy.
func TestThemesShareTheSameLabels(t *testing.T) {
	themes, err := fs.ReadDir(files, "files/themes")
	if err != nil {
		t.Fatal(err)
	}
	read := func(theme, file string) string {
		b, err := fs.ReadFile(files, "files/themes/"+theme+"/i18n/"+file)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	keys := func(toml string) []string {
		var out []string
		for _, line := range strings.Split(strings.TrimSpace(toml), "\n") {
			key, _, _ := strings.Cut(line, "=")
			out = append(out, strings.TrimSpace(key))
		}
		return out
	}

	reference := themes[0].Name()
	if read(reference, "ja.toml") != read(reference, "ja-jp.toml") {
		t.Errorf("%s: ja.toml and ja-jp.toml differ", reference)
	}
	if got, want := keys(read(reference, "ja.toml")), keys(read(reference, "en.toml")); !slices.Equal(got, want) {
		t.Errorf("%s: Japanese labels %v, English labels %v", reference, got, want)
	}
	for _, theme := range themes[1:] {
		for _, file := range []string{"en.toml", "ja.toml", "ja-jp.toml"} {
			if read(theme.Name(), file) != read(reference, file) {
				t.Errorf("%s/i18n/%s differs from %s's", theme.Name(), file, reference)
			}
		}
	}
}
