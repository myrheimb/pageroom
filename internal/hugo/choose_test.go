package hugo

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		out      string
		version  string
		extended bool
	}{
		{"hugo v0.168.0+extended+withdeploy darwin/arm64 BuildDate=unknown VendorInfo=nixpkgs", "0.168.0", true},
		{"hugo v0.150.1-abc123+extended linux/amd64 BuildDate=2025-01-01", "0.150.1", true},
		{"hugo v0.140.0 windows/amd64", "0.140.0", false},
	}
	for _, c := range cases {
		v, ext, ok := parseVersion(c.out)
		if !ok || v != c.version || ext != c.extended {
			t.Errorf("%q → %s %v %v", c.out, v, ext, ok)
		}
	}
}

func TestChoose(t *testing.T) {
	m := &Manager{CacheDir: t.TempDir()}
	mine := []Installed{{Path: "/nix/hugo", Version: "0.168.0", Extended: true}}

	// The user's own Hugo wins when it's the right version.
	if c := m.Choose("0.168.0", mine); c.Path != "/nix/hugo" || c.Source != "installed" {
		t.Fatalf("%+v", c)
	}
	// Wrong version and nothing cached: a download is needed, and we know
	// what the user has so the UI can explain.
	c := m.Choose("0.170.0", mine)
	if c.Path != "" || c.Installed == nil || c.Installed.Version != "0.168.0" {
		t.Fatalf("%+v", c)
	}
	// Once Pageroom has its own copy, that's used, with the note info kept.
	_ = os.MkdirAll(filepath.Join(m.CacheDir, "0.170.0"), 0o755)
	_ = os.WriteFile(m.CachedPath("0.170.0"), []byte("bin"), 0o755)
	if c := m.Choose("0.170.0", mine); c.Source != "pageroom" || c.Installed == nil {
		t.Fatalf("%+v", c)
	}
	// A non-extended install of the right version isn't enough (most themes need SCSS).
	plain := []Installed{{Path: "/usr/bin/hugo", Version: "0.169.0"}}
	if c := m.Choose("0.169.0", plain); c.Source == "installed" {
		t.Fatalf("non-extended Hugo chosen: %+v", c)
	}
}

func TestPickAssetMacPkgOnly(t *testing.T) {
	sums := "aa  hugo_extended_0.168.0_darwin-universal.pkg\nbb  hugo_extended_0.168.0_linux-amd64.tar.gz\n"
	name, sum, err := pickAsset(sums, "0.168.0", "darwin", "arm64")
	if err != nil || name != "hugo_extended_0.168.0_darwin-universal.pkg" || sum != "aa" {
		t.Fatalf("%s %s %v", name, sum, err)
	}
}

// fakePkg builds a minimal xar package with a gzip+cpio (odc) payload,
// shaped like Hugo's real releases.
func fakePkg(t *testing.T, bin []byte) []byte {
	t.Helper()
	var cpio bytes.Buffer
	entry := func(name string, mode int64, data []byte) {
		fmt.Fprintf(&cpio, "070707%06o%06o%06o%06o%06o%06o%06o%011o%06o%011o%s\x00", 0, 0, mode, 0, 0, 1, 0, 0, len(name)+1, len(data), name)
		cpio.Write(data)
	}
	entry(".", 0o40755, nil)
	entry("./._hugo", 0o100644, []byte("apple double"))
	entry("./hugo", 0o100755, bin)
	entry("TRAILER!!!", 0, nil)
	var payload bytes.Buffer
	gz := gzip.NewWriter(&payload)
	_, _ = gz.Write(cpio.Bytes())
	_ = gz.Close()

	toc := fmt.Sprintf(`<?xml version="1.0"?><xar><toc><file id="1"><data><length>%d</length><offset>20</offset><size>%d</size><encoding style="application/octet-stream"/></data><type>file</type><name>Payload</name></file></toc></xar>`, payload.Len(), payload.Len())
	var ztoc bytes.Buffer
	zw := zlib.NewWriter(&ztoc)
	_, _ = zw.Write([]byte(toc))
	_ = zw.Close()

	var out bytes.Buffer
	out.WriteString("xar!")
	_ = binary.Write(&out, binary.BigEndian, uint16(28))
	_ = binary.Write(&out, binary.BigEndian, uint16(1))
	_ = binary.Write(&out, binary.BigEndian, uint64(ztoc.Len()))
	_ = binary.Write(&out, binary.BigEndian, uint64(len(toc)))
	_ = binary.Write(&out, binary.BigEndian, uint32(1))
	out.Write(ztoc.Bytes())
	out.Write(make([]byte, 20)) // checksum area at heap offset 0
	out.Write(payload.Bytes())
	return out.Bytes()
}

func TestExtractPkg(t *testing.T) {
	want := []byte("#!/bin/sh\necho hugo\n")
	got, err := extract(fakePkg(t, want), "hugo_extended_0.168.0_darwin-universal.pkg")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := extractPkg([]byte("not a package at all, definitely not")); err == nil {
		t.Fatal("accepted garbage")
	}
}
