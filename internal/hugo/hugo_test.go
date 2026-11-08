package hugo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSiteVersion(t *testing.T) {
	root := t.TempDir()
	if v := SiteVersion(root, ""); v != DefaultVersion {
		t.Fatalf("default %s", v)
	}
	os.WriteFile(filepath.Join(root, "netlify.toml"), []byte("[build.environment]\n  HUGO_VERSION = \"0.140.1\"\n"), 0o644)
	if v := SiteVersion(root, ""); v != "0.140.1" {
		t.Fatalf("netlify %s", v)
	}
	if v := SiteVersion(root, "v0.150.0"); v != "0.150.0" {
		t.Fatalf("prefs %s", v)
	}
}

func TestPickAsset(t *testing.T) {
	sums := "aaa  hugo_0.168.0_linux-amd64.tar.gz\nbbb  hugo_extended_0.168.0_linux-amd64.tar.gz\nccc  hugo_extended_0.168.0_windows-amd64.zip\nddd  hugo_extended_0.168.0_darwin-universal.tar.gz\n"
	cases := map[[2]string]string{
		{"linux", "amd64"}:   "hugo_extended_0.168.0_linux-amd64.tar.gz",
		{"windows", "amd64"}: "hugo_extended_0.168.0_windows-amd64.zip",
		{"darwin", "arm64"}:  "hugo_extended_0.168.0_darwin-universal.tar.gz",
	}
	for in, want := range cases {
		got, _, err := pickAsset(sums, "0.168.0", in[0], in[1])
		if err != nil || got != want {
			t.Errorf("%v: %s %v", in, got, err)
		}
	}
	if _, _, err := pickAsset(sums, "0.168.0", "linux", "riscv64"); err == nil {
		t.Error("expected no asset")
	}
}

func fakeRelease(t *testing.T, corrupt bool) *httptest.Server {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	bin := []byte("#!/bin/sh\necho hugo v0.168.0-extended\n")
	tw.WriteHeader(&tar.Header{Name: "../evil", Mode: 0o755, Size: 1, Typeflag: tar.TypeReg})
	tw.Write([]byte("x"))
	tw.WriteHeader(&tar.Header{Name: binName(), Mode: 0o755, Size: int64(len(bin)), Typeflag: tar.TypeReg})
	tw.Write(bin)
	tw.Close()
	gz.Close()
	sum := sha256.Sum256(archive.Bytes())
	hexsum := hex.EncodeToString(sum[:])
	if corrupt {
		hexsum = "00" + hexsum[2:]
	}
	asset := fmt.Sprintf("hugo_extended_0.168.0_%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		asset = "hugo_extended_0.168.0_darwin-universal.tar.gz"
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0.168.0/hugo_0.168.0_checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", hexsum, asset)
		case "/v0.168.0/" + asset:
			w.Write(archive.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestDownloadVerifiesChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tar fixture")
	}
	srv := fakeRelease(t, false)
	defer srv.Close()
	m := &Manager{CacheDir: t.TempDir(), DownloadURL: srv.URL}
	dest := filepath.Join(m.CacheDir, "0.168.0", binName())
	if err := m.download(context.Background(), "0.168.0", dest); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(dest)
	if err != nil || st.Mode().Perm()&0o100 == 0 {
		t.Fatalf("binary missing or not executable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(m.CacheDir, "evil")); err == nil {
		t.Fatal("path traversal entry extracted")
	}

	bad := fakeRelease(t, true)
	defer bad.Close()
	m2 := &Manager{CacheDir: t.TempDir(), DownloadURL: bad.URL}
	if err := m2.download(context.Background(), "0.168.0", filepath.Join(m2.CacheDir, "h")); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}
