package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeJoinRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{"../x", "a/../../x", "/etc/passwd", "..", ""} {
		if _, err := SafeJoin(root, bad); err == nil {
			t.Errorf("SafeJoin(%q) should fail", bad)
		}
	}
	p, err := SafeJoin(root, "content/posts/a.md")
	if err != nil || !strings.HasPrefix(p, root) {
		t.Fatalf("got %q, %v", p, err)
	}
}

func TestSafeJoinRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unsupported")
	}
	if _, err := SafeJoin(root, "link/evil.txt"); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestWriteFileAtomicPreservesPerm(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(p, []byte("old"), 0o600)
	if err := WriteFileAtomic(p, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	st, _ := os.Stat(p)
	if string(b) != "new" || st.Mode().Perm() != 0o600 {
		t.Fatalf("content %q perm %v", b, st.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

func TestCreateExclusiveNeverOverwrites(t *testing.T) {
	p := filepath.Join(t.TempDir(), "img.jpg")
	if err := CreateExclusive(p, strings.NewReader("one")); err != nil {
		t.Fatal(err)
	}
	if err := CreateExclusive(p, strings.NewReader("two")); !os.IsExist(err) {
		t.Fatalf("expected ErrExist, got %v", err)
	}
	if got := filepath.Base(UniquePath(p)); got != "img-2.jpg" {
		t.Fatalf("UniquePath = %s", got)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello, World!":           "hello-world",
		"Bjørnar's Summer Trip":   "bjornars-summer-trip",
		"  Crème brûlée  recipe ": "creme-brulee-recipe",
		"!!!":                     "untitled",
		"Ærlig talt":              "aerlig-talt",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
