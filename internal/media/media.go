// Package media imports images into a site safely: metadata such as location
// is removed, large photos are scaled down, unsafe SVGs are refused, and an
// existing file is never overwritten.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/myrheimb/pageroom/internal/fsutil"
)

const MaxUpload = 60 << 20

// DefaultMaxWidth keeps photos sharp on large screens without 20 MB uploads.
const DefaultMaxWidth = 2400

type Item struct {
	Path     string    `json:"path"` // relative to site root
	Name     string    `json:"name"`
	Ref      string    `json:"ref"` // what to write in Markdown
	Page     string    `json:"page,omitempty"`
	Width    int       `json:"width"`
	Height   int       `json:"height"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Format   string    `json:"format"`
}

type ImportResult struct {
	Item
	Stripped bool   `json:"stripped"` // metadata (e.g. GPS) was removed
	Resized  bool   `json:"resized"`
	Note     string `json:"note,omitempty"`
}

type Options struct {
	MaxWidth int
}

var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".svg": true, ".avif": true}

func IsImage(p string) bool { return imageExts[strings.ToLower(path.Ext(p))] }

var fontTypes = map[string]string{".woff2": "font/woff2", ".woff": "font/woff", ".ttf": "font/ttf", ".otf": "font/otf"}

// FontType returns the content type of a font file, or "" when the path
// is not a font.
func FontType(p string) string { return fontTypes[strings.ToLower(path.Ext(p))] }

// Process cleans image data. It returns the bytes to store and the
// canonical extension.
func Process(data []byte, opt Options) (out []byte, ext string, res ImportResult, err error) {
	if len(data) > MaxUpload {
		return nil, "", res, fmt.Errorf("this file is larger than %d MB", MaxUpload>>20)
	}
	if opt.MaxWidth <= 0 {
		opt.MaxWidth = DefaultMaxWidth
	}
	switch sniff(data) {
	case "jpeg":
		stripped, orient, err := stripJPEG(data)
		if err != nil {
			return nil, "", res, errors.New("this JPEG file seems to be damaged")
		}
		res.Stripped = len(stripped) != len(data)
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(stripped))
		if err != nil {
			return nil, "", res, errors.New("this JPEG file seems to be damaged")
		}
		res.Width, res.Height = cfg.Width, cfg.Height
		if orient == 1 && cfg.Width <= opt.MaxWidth {
			return stripped, ".jpg", res, nil // lossless path
		}
		img, err := jpeg.Decode(bytes.NewReader(stripped))
		if err != nil {
			return nil, "", res, err
		}
		img = orientImage(img, orient)
		img, res.Resized = fit(img, opt.MaxWidth)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
			return nil, "", res, err
		}
		b := img.Bounds()
		res.Width, res.Height = b.Dx(), b.Dy()
		return buf.Bytes(), ".jpg", res, nil
	case "png":
		stripped, err := stripPNG(data)
		if err != nil {
			return nil, "", res, errors.New("this PNG file seems to be damaged")
		}
		res.Stripped = len(stripped) != len(data)
		cfg, err := png.DecodeConfig(bytes.NewReader(stripped))
		if err != nil {
			return nil, "", res, errors.New("this PNG file seems to be damaged")
		}
		res.Width, res.Height = cfg.Width, cfg.Height
		if cfg.Width <= opt.MaxWidth {
			return stripped, ".png", res, nil
		}
		img, err := png.Decode(bytes.NewReader(stripped))
		if err != nil {
			return nil, "", res, err
		}
		img, res.Resized = fit(img, opt.MaxWidth)
		var buf bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
			return nil, "", res, err
		}
		b := img.Bounds()
		res.Width, res.Height = b.Dx(), b.Dy()
		return buf.Bytes(), ".png", res, nil
	case "gif":
		cfg, err := gif.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, "", res, errors.New("this GIF file seems to be damaged")
		}
		res.Width, res.Height = cfg.Width, cfg.Height
		return data, ".gif", res, nil // keep animation intact
	case "webp":
		stripped, err := stripWebP(data)
		if err != nil {
			return nil, "", res, errors.New("this WebP file seems to be damaged")
		}
		res.Stripped = len(stripped) != len(data)
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(stripped)); err == nil {
			res.Width, res.Height = cfg.Width, cfg.Height
		}
		return stripped, ".webp", res, nil
	case "svg":
		if reason := unsafeSVG(data); reason != "" {
			return nil, "", res, fmt.Errorf("this SVG can't be added because it contains %s, which could run code on your website", reason)
		}
		return data, ".svg", res, nil
	case "heic":
		return nil, "", res, errors.New("HEIC photos (from iPhones) aren't supported by web browsers. Please export it as JPEG first (on iPhone: Settings → Camera → Formats → Most Compatible)")
	}
	return nil, "", res, errors.New("this file isn't a supported image (JPEG, PNG, GIF, WebP or SVG)")
}

func sniff(b []byte) string {
	switch {
	case len(b) > 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "jpeg"
	case bytes.HasPrefix(b, pngSig):
		return "png"
	case bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a")):
		return "gif"
	case len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "webp"
	case len(b) > 12 && string(b[4:8]) == "ftyp" && (strings.HasPrefix(string(b[8:12]), "hei") || strings.HasPrefix(string(b[8:12]), "mif")):
		return "heic"
	}
	head := strings.ToLower(string(b[:min(len(b), 1024)]))
	if strings.Contains(head, "<svg") {
		return "svg"
	}
	return ""
}

var svgDanger = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`(?i)<\s*script`), "a script"},
	{regexp.MustCompile(`(?i)\son[a-z]+\s*=`), "an event handler"},
	{regexp.MustCompile(`(?i)javascript\s*:`), "a javascript: link"},
	{regexp.MustCompile(`(?i)<\s*foreignObject`), "embedded web content"},
	{regexp.MustCompile(`(?i)<\s*(iframe|embed|object)`), "embedded content"},
	{regexp.MustCompile(`(?i)<!ENTITY`), "an XML entity"},
}

func unsafeSVG(b []byte) string {
	for _, d := range svgDanger {
		if d.re.Match(b) {
			return d.reason
		}
	}
	return ""
}

func fit(img image.Image, maxW int) (image.Image, bool) {
	b := img.Bounds()
	if b.Dx() <= maxW {
		return img, false
	}
	h := b.Dy() * maxW / b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, maxW, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst, true
}

// orientImage applies an EXIF orientation so the photo shows upright
// after the EXIF block is gone.
func orientImage(src image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	swap := o >= 5
	dw, dh := w, h
	if swap {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

type Library struct {
	Root       string // site root
	ContentDir string // "content"
	StaticDir  string // folder for images not tied to a bundle, e.g. "static/images"
	MaxWidth   int
	TrashDir   string
}

// Import processes src and stores it next to the page (for bundles) or in
// the shared images folder. Existing files are never replaced.
func (l *Library) Import(data []byte, originalName, page string) (*ImportResult, error) {
	out, ext, res, err := Process(data, Options{MaxWidth: l.MaxWidth})
	if err != nil {
		return nil, err
	}
	stem := strings.TrimSuffix(filepath.Base(originalName), filepath.Ext(originalName))
	name := fsutil.Slugify(stem) + ext

	var dirRel string
	bundle := page != "" && strings.HasPrefix(path.Base(page), "index.")
	if bundle {
		dirRel = path.Dir(page)
	} else {
		dirRel = l.StaticDir
	}
	dir, err := fsutil.SafeJoin(l.Root, dirRel)
	if err != nil {
		return nil, err
	}
	var full string
	for attempt := 0; attempt < 50; attempt++ {
		full = fsutil.UniquePath(filepath.Join(dir, name))
		err = fsutil.CreateExclusive(full, bytes.NewReader(out))
		if !errors.Is(err, os.ErrExist) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	rel, _ := filepath.Rel(l.Root, full)
	res.Item = l.item(filepath.ToSlash(rel), res.Width, res.Height)
	if res.Stripped {
		res.Note = "Hidden information such as camera details and location was removed."
	}
	if res.Resized {
		res.Note = strings.TrimSpace(res.Note + fmt.Sprintf(" Resized to %d pixels wide for faster pages.", res.Width))
	}
	return &res, nil
}

func (l *Library) item(rel string, w, h int) Item {
	it := Item{Path: rel, Name: path.Base(rel), Width: w, Height: h, Format: strings.TrimPrefix(path.Ext(rel), ".")}
	if st, err := os.Stat(filepath.Join(l.Root, filepath.FromSlash(rel))); err == nil {
		it.Size, it.Modified = st.Size(), st.ModTime()
	}
	switch {
	case strings.HasPrefix(rel, l.ContentDir+"/"):
		// page bundle resource: referenced relative to the page
		it.Ref = path.Base(rel)
		dir := path.Dir(rel)
		for _, idx := range []string{"index.md", "index.markdown", "_index.md"} {
			if fsutil.Exists(filepath.Join(l.Root, filepath.FromSlash(dir), idx)) {
				it.Page = dir + "/" + idx
				break
			}
		}
	case strings.HasPrefix(rel, "static/"):
		it.Ref = "/" + strings.TrimPrefix(rel, "static/")
	case strings.HasPrefix(rel, "assets/"):
		it.Ref = strings.TrimPrefix(rel, "assets/")
	default:
		it.Ref = "/" + rel
	}
	return it
}

func (l *Library) List() ([]Item, error) {
	var out []Item
	for _, top := range []string{l.ContentDir, "static", "assets"} {
		root := filepath.Join(l.Root, filepath.FromSlash(top))
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if d.IsDir() || !IsImage(p) {
				return nil
			}
			rel, _ := filepath.Rel(l.Root, p)
			it := l.item(filepath.ToSlash(rel), 0, 0)
			if f, err := os.Open(p); err == nil {
				if cfg, _, err := image.DecodeConfig(f); err == nil {
					it.Width, it.Height = cfg.Width, cfg.Height
				}
				_ = f.Close()
			}
			out = append(out, it)
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out, nil
}

// Usage lists content files that mention the image, so the UI can warn
// before deleting something that's in use.
func (l *Library) Usage(rel string) []string {
	needle := path.Base(rel)
	var out []string
	root := filepath.Join(l.Root, filepath.FromSlash(l.ContentDir))
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".md" && ext != ".markdown" && ext != ".html" {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte(needle)) {
			r, _ := filepath.Rel(l.Root, p)
			out = append(out, filepath.ToSlash(r))
		}
		return nil
	})
	return out
}

func (l *Library) Delete(rel string) error {
	if !IsImage(rel) {
		return errors.New("only images can be deleted here")
	}
	src, err := fsutil.SafeJoin(l.Root, rel)
	if err != nil {
		return err
	}
	dest := filepath.Join(l.TrashDir, time.Now().Format("20060102-150405"), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dest)
}
