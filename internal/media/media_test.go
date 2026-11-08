package media

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testJPEG(t *testing.T, w, h int, orientation uint16, gps bool) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var raw bytes.Buffer
	jpeg.Encode(&raw, img, nil)
	b := raw.Bytes()

	// Build an EXIF APP1 with an orientation tag (and fake GPS text).
	var tiff bytes.Buffer
	tiff.WriteString("II*\x00")
	binary.Write(&tiff, binary.LittleEndian, uint32(8))
	binary.Write(&tiff, binary.LittleEndian, uint16(1))
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0112))
	binary.Write(&tiff, binary.LittleEndian, uint16(3))
	binary.Write(&tiff, binary.LittleEndian, uint32(1))
	binary.Write(&tiff, binary.LittleEndian, orientation)
	tiff.Write([]byte{0, 0})
	binary.Write(&tiff, binary.LittleEndian, uint32(0))
	if gps {
		tiff.WriteString("GPS 59.9139N 10.7522E")
	}
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)
	out := append([]byte{}, b[:2]...)
	out = append(out, seg...)
	return append(out, b[2:]...)
}

func TestJPEGMetadataStrippedLosslessly(t *testing.T) {
	src := testJPEG(t, 40, 20, 1, true)
	out, ext, res, err := Process(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if ext != ".jpg" || !res.Stripped || res.Resized {
		t.Fatalf("ext %s res %+v", ext, res)
	}
	if bytes.Contains(out, []byte("GPS")) || bytes.Contains(out, []byte("Exif")) {
		t.Fatal("metadata still present")
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("result not decodable: %v", err)
	}
}

func TestJPEGOrientationApplied(t *testing.T) {
	src := testJPEG(t, 40, 20, 6, false) // rotated 90°: stored landscape, shown portrait
	out, _, res, err := Process(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := jpeg.DecodeConfig(bytes.NewReader(out))
	if cfg.Width != 20 || cfg.Height != 40 || res.Width != 20 {
		t.Fatalf("not rotated: %dx%d", cfg.Width, cfg.Height)
	}
}

func TestResize(t *testing.T) {
	src := testJPEG(t, 300, 100, 1, false)
	out, _, res, err := Process(src, Options{MaxWidth: 150})
	if err != nil || !res.Resized {
		t.Fatalf("%+v %v", res, err)
	}
	cfg, _ := jpeg.DecodeConfig(bytes.NewReader(out))
	if cfg.Width != 150 || cfg.Height != 50 {
		t.Fatalf("%dx%d", cfg.Width, cfg.Height)
	}
}

func TestPNGTextChunksRemoved(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	b := buf.Bytes()
	ihdrEnd := 8 + 12 + 13
	withText := append(append(append([]byte{}, b[:ihdrEnd]...), pngChunk("tEXt", []byte("Author\x00Secret Person"))...), b[ihdrEnd:]...)
	out, _, res, err := Process(withText, Options{})
	if err != nil || !res.Stripped || bytes.Contains(out, []byte("Secret")) {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Fatal(err)
	}
}

func TestUnsafeSVGRefused(t *testing.T) {
	for _, svg := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		`<svg onload="alert(1)"></svg>`,
		`<svg><a href="javascript:alert(1)"/></svg>`,
	} {
		if _, _, _, err := Process([]byte(svg), Options{}); err == nil {
			t.Errorf("accepted %s", svg)
		}
	}
	if _, ext, _, err := Process([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>`), Options{}); err != nil || ext != ".svg" {
		t.Fatal(err)
	}
}

func TestRejectsNonImages(t *testing.T) {
	if _, _, _, err := Process([]byte("MZ\x90\x00 definitely an exe"), Options{}); err == nil {
		t.Fatal("accepted non-image")
	}
}

func TestLibraryImportPlacesAndNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "content/posts/trip"), 0o755)
	os.WriteFile(filepath.Join(root, "content/posts/trip/index.md"), []byte("![](Beach Day.jpg)"), 0o644)
	l := &Library{Root: root, ContentDir: "content", StaticDir: "static/images", TrashDir: t.TempDir()}
	img := testJPEG(t, 10, 10, 1, false)

	a, err := l.Import(img, "Beach Day.JPG", "content/posts/trip/index.md")
	if err != nil {
		t.Fatal(err)
	}
	if a.Path != "content/posts/trip/beach-day.jpg" || a.Ref != "beach-day.jpg" || a.Page != "content/posts/trip/index.md" {
		t.Fatalf("%+v", a.Item)
	}
	b, _ := l.Import(img, "Beach Day.JPG", "content/posts/trip/index.md")
	if b.Path == a.Path {
		t.Fatal("overwrote existing image")
	}
	c, _ := l.Import(img, "logo.jpg", "content/about.md")
	if c.Path != "static/images/logo.jpg" || c.Ref != "/images/logo.jpg" {
		t.Fatalf("%+v", c.Item)
	}
	items, _ := l.List()
	if len(items) != 3 {
		t.Fatalf("list: %d", len(items))
	}
	if err := l.Delete(c.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, c.Path)); !os.IsNotExist(err) {
		t.Fatal("not deleted")
	}
	if err := l.Delete("content/posts/trip/index.md"); err == nil || !strings.Contains(err.Error(), "only images") {
		t.Fatal("deleted a page via media library")
	}
}

// pngChunk builds a PNG chunk.
func pngChunk(typ string, data []byte) []byte {
	var c bytes.Buffer
	binary.Write(&c, binary.BigEndian, uint32(len(data)))
	c.WriteString(typ)
	c.Write(data)
	binary.Write(&c, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
	return c.Bytes()
}
