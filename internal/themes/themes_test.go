package themes

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var commitTime = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

type entry struct {
	name, body string
	kind       byte
}

func archiveOf(t *testing.T, entries ...entry) *bytes.Reader {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(gz)
	for _, item := range entries {
		header := &tar.Header{Name: item.name, Typeflag: item.kind, Mode: 0o644, ModTime: commitTime}
		switch item.kind {
		case tar.TypeReg:
			header.Size = int64(len(item.body))
		case tar.TypeSymlink:
			header.Linkname = item.body
		}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if item.kind == tar.TypeReg {
			_, _ = archive.Write([]byte(item.body))
		}
	}
	_ = archive.Close()
	_ = gz.Close()
	return bytes.NewReader(buffer.Bytes())
}

func file(name, body string) entry { return entry{name, body, tar.TypeReg} }

func extracted(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	_ = filepath.WalkDir(dir, func(path string, item os.DirEntry, err error) error {
		if err == nil && !item.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			names = append(names, filepath.ToSlash(rel))
		}
		return err
	})
	return names
}

func TestExtractKeepsOnlyThemeFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "paper")
	_, updated, err := Extract(archiveOf(t,
		entry{"paper-abc/", "", tar.TypeDir},
		file("paper-abc/theme.toml", `name = "Paper"`),
		file("paper-abc/layouts/index.html", "<h1>hi</h1>"),
		file("paper-abc/exampleSite/hugo.toml", "big"),
		entry{"paper-abc/layouts/link.html", "/etc/passwd", tar.TypeSymlink},
	), "", dir)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(extracted(t, dir), ",")
	if got != "layouts/index.html,theme.toml" {
		t.Errorf("extracted %s", got)
	}
	if !updated.Equal(commitTime) {
		t.Errorf("updated = %v", updated)
	}
}

func TestExtractSubdir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "blox")
	_, _, err := Extract(archiveOf(t,
		file("repo-abc/README.md", "readme"),
		file("repo-abc/modules/blox/theme.toml", `name = "Blox"`),
	), "modules/blox", dir)
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(extracted(t, dir), ","); got != "theme.toml" {
		t.Errorf("extracted %s", got)
	}
}

func TestExtractRejects(t *testing.T) {
	for name, archive := range map[string]*bytes.Reader{
		"escape": archiveOf(t, file("repo-abc/../../evil.sh", "x")),
		"empty":  archiveOf(t, file("repo-abc/exampleSite/hugo.toml", "x")),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Extract(archive, "", filepath.Join(t.TempDir(), "theme")); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestExtractHashIgnoresArchiveOrder(t *testing.T) {
	first, _, err := Extract(archiveOf(t, file("r-a/a.html", "a"), file("r-a/b.html", "b")), "", filepath.Join(t.TempDir(), "one"))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := Extract(archiveOf(t, file("r-b/b.html", "b"), file("r-b/a.html", "a")), "", filepath.Join(t.TempDir(), "two"))
	if err != nil {
		t.Fatal(err)
	}
	changed, _, err := Extract(archiveOf(t, file("r-c/a.html", "a"), file("r-c/b.html", "B")), "", filepath.Join(t.TempDir(), "three"))
	if err != nil {
		t.Fatal(err)
	}

	if first != second || first == changed {
		t.Errorf("hashes: %s %s %s", first, second, changed)
	}
}

type serve []byte

func (body serve) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Request: request}, nil
}

func TestInstallOnlyKeepsTheTestedVersion(t *testing.T) {
	archive := func() *bytes.Reader { return archiveOf(t, file("paper-abc/theme.toml", `name = "Paper"`)) }
	tested, _, err := Extract(archive(), "", filepath.Join(t.TempDir(), "tested"))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(archive())
	client := &http.Client{Transport: serve(data)}
	themesDir := t.TempDir()

	tampered := Theme{Archive: "https://example.com/paper.tar.gz", SHA256: "not-" + tested}
	if err := Install(context.Background(), client, tampered, filepath.Join(themesDir, "paper")); err == nil {
		t.Fatal("expected a mismatched download to be refused")
	}
	if leftovers, _ := os.ReadDir(themesDir); len(leftovers) != 0 {
		t.Fatalf("a refused download left %v behind", leftovers)
	}

	good := Theme{Archive: tampered.Archive, SHA256: tested}
	if err := Install(context.Background(), client, good, filepath.Join(themesDir, "paper")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(extracted(t, themesDir), ","); got != "paper/theme.toml" {
		t.Errorf("installed %s", got)
	}
}

func TestCatalogLoads(t *testing.T) {
	index, err := Catalog()
	if err != nil || len(index.Themes) == 0 {
		t.Fatalf("%d themes, %v", len(index.Themes), err)
	}
}
