// Package appearance turns style choices (colors, fonts, corner roundness)
// and custom CSS into a stylesheet, and hooks it into the site's Hugo theme
// without editing the theme.
package appearance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/myrheimb/pageroom/internal/fsutil"
)

const MaxRadius = 24

type Tokens struct {
	Primary      string `json:"primary"`
	Background   string `json:"background"`
	Text         string `json:"text"`
	Heading      string `json:"heading"`
	HeadingFont  string `json:"headingFont"`
	BodyFont     string `json:"bodyFont"`
	FontSize     int    `json:"fontSize"`     // px, base
	ContentWidth int    `json:"contentWidth"` // px, 0 = theme default
	Radius       int    `json:"radius"`       // px, -1 = theme default, 0 = square corners

	// Fonts are font files stored on the site (under static/), which
	// HeadingFont and BodyFont may name by family.
	Fonts []SiteFont `json:"fonts,omitempty"`
}

type SiteFont struct {
	Family   string     `json:"family"`
	Fallback string     `json:"fallback"` // FontStacks key used when the files fail to load. Empty means "system".
	Files    []FontFile `json:"files"`
}

type FontFile struct {
	Path         string `json:"path"`                   // under static/, such as fonts/manrope.woff2
	Weight       string `json:"weight"`                 // "400" or a range such as "200 800"
	Style        string `json:"style"`                  // normal, italic or oblique
	UnicodeRange string `json:"unicodeRange,omitempty"` // such as "U+0000-00FF"
}

type Style struct {
	Tokens    Tokens `json:"tokens"`
	CustomCSS string `json:"customCSS"`
}

// FontStacks are curated font choices that use fonts already on visitors'
// devices, so nothing loads from third parties.
var FontStacks = map[string]string{
	"system":       `system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`,
	"humanist":     `Seravek, "Gill Sans Nova", Ubuntu, Calibri, "DejaVu Sans", source-sans-pro, sans-serif`,
	"geometric":    `Avenir, Montserrat, Corbel, "URW Gothic", source-sans-pro, sans-serif`,
	"classical":    `Optima, Candara, "Noto Sans", source-sans-pro, sans-serif`,
	"oldstyle":     `"Iowan Old Style", "Palatino Linotype", "URW Palladio L", P052, serif`,
	"transitional": `Charter, "Bitstream Charter", "Sitka Text", Cambria, serif`,
	"didone":       `Didot, "Bodoni MT", "Noto Serif Display", "URW Palladio L", P052, Sylfaen, serif`,
	"slab":         `Rockwell, "Rockwell Nova", "Roboto Slab", "DejaVu Serif", "Sitka Small", serif`,
	"rounded":      `ui-rounded, "Hiragino Maru Gothic ProN", Quicksand, Comfortaa, Manjari, "Arial Rounded MT", Calibri, source-sans-pro, sans-serif`,
	"mono":         `ui-monospace, "Cascadia Code", "Source Code Pro", Menlo, Consolas, "DejaVu Sans Mono", monospace`,
}

const (
	beginMarker = "/* >>> Pageroom styles (managed by Pageroom; edit in Appearance) >>> */"
	endMarker   = "/* <<< Pageroom styles <<< */"
	customMark  = "/* --- Custom CSS --- */"
	tokensPfx   = "/* pageroom-tokens: "
)

var (
	colorRe        = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	familyRe       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _-]*$`)
	fontPathRe     = regexp.MustCompile(`^fonts/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	fontWeightRe   = regexp.MustCompile(`^[1-9][0-9]{2}(?: [1-9][0-9]{2})?$`)
	unicodeRangeRe = regexp.MustCompile(`^U\+[0-9A-Fa-f?]{1,6}(?:-[0-9A-Fa-f]{1,6})?(?:, ?U\+[0-9A-Fa-f?]{1,6}(?:-[0-9A-Fa-f]{1,6})?)*$`)
)

func (f SiteFont) validate() error {
	if !familyRe.MatchString(f.Family) {
		return fmt.Errorf("invalid font family %q", f.Family)
	}
	if _, builtIn := FontStacks[f.Family]; builtIn {
		return fmt.Errorf("font family %q clashes with a built-in font", f.Family)
	}
	if _, ok := FontStacks[f.Fallback]; f.Fallback != "" && !ok {
		return fmt.Errorf("unknown fallback font %q", f.Fallback)
	}
	for _, file := range f.Files {
		switch {
		case !fontPathRe.MatchString(file.Path) || strings.Contains(file.Path, ".."):
			return fmt.Errorf("invalid font file path %q", file.Path)
		case !fontWeightRe.MatchString(file.Weight):
			return fmt.Errorf("invalid font weight %q", file.Weight)
		case file.Style != "normal" && file.Style != "italic" && file.Style != "oblique":
			return fmt.Errorf("invalid font style %q", file.Style)
		case file.UnicodeRange != "" && !unicodeRangeRe.MatchString(file.UnicodeRange):
			return fmt.Errorf("invalid unicode range %q", file.UnicodeRange)
		}
	}
	return nil
}

// stack is the font-family value for a font name: a built-in stack, or a
// site font followed by its fallback. Unknown names give "".
func (t Tokens) stack(name string) string {
	if stack, ok := FontStacks[name]; ok {
		return stack
	}
	for _, font := range t.Fonts {
		if font.Family == name {
			fallback := font.Fallback
			if fallback == "" {
				fallback = "system"
			}

			return fmt.Sprintf("%q, %s", font.Family, FontStacks[fallback])
		}
	}

	return ""
}

var fontFormats = map[string]string{".otf": "opentype", ".ttf": "truetype", ".woff": "woff", ".woff2": "woff2"}

// fontFaces renders @font-face rules for the site fonts in use. URLs are
// relative to the stylesheet, so they work when the site is in a subfolder.
func (t Tokens) fontFaces() string {
	var b strings.Builder
	for _, font := range t.Fonts {
		if font.Family != t.HeadingFont && font.Family != t.BodyFont {
			continue
		}
		for _, file := range font.Files {
			format := fontFormats[strings.ToLower(filepath.Ext(file.Path))]
			fmt.Fprintf(&b, "@font-face {\n  font-family: %q;\n  src: url(\"../%s\") format(%q);\n  font-weight: %s;\n  font-style: %s;\n  font-display: swap;\n", font.Family, file.Path, format, file.Weight, file.Style)
			if file.UnicodeRange != "" {
				fmt.Fprintf(&b, "  unicode-range: %s;\n", file.UnicodeRange)
			}
			b.WriteString("}\n")
		}
	}

	return b.String()
}

// Validate rejects values that could break out of the stylesheet.
func (t Tokens) Validate() error {
	for name, c := range map[string]string{"primary": t.Primary, "background": t.Background, "text": t.Text, "heading": t.Heading} {
		if c != "" && !colorRe.MatchString(c) {
			return fmt.Errorf("%s color must look like #1a2b3c", name)
		}
	}
	for _, font := range t.Fonts {
		if err := font.validate(); err != nil {
			return err
		}
	}
	for _, f := range []string{t.HeadingFont, t.BodyFont} {
		if f != "" && t.stack(f) == "" {
			return fmt.Errorf("unknown font %q", f)
		}
	}
	if t.FontSize != 0 && (t.FontSize < 12 || t.FontSize > 28) {
		return errors.New("text size must be between 12 and 28")
	}
	if t.ContentWidth != 0 && (t.ContentWidth < 480 || t.ContentWidth > 2000) {
		return errors.New("content width must be between 480 and 2000")
	}
	if t.Radius < -1 || t.Radius > MaxRadius {
		return fmt.Errorf("rounded corners must be between 0 and %d", MaxRadius)
	}
	return nil
}

// CSS renders the tokens. Only chosen values produce rules, so an untouched
// Style panel leaves the theme exactly as designed.
func (t Tokens) CSS() string {
	var vars, rules []string
	add := func(v, val string) {
		if val != "" {
			vars = append(vars, fmt.Sprintf("  --pr-%s: %s;", v, val))
		}
	}
	add("primary", t.Primary)
	add("bg", t.Background)
	add("text", t.Text)
	add("heading", t.Heading)
	add("heading-font", t.stack(t.HeadingFont))
	add("body-font", t.stack(t.BodyFont))
	if len(vars) == 0 && t.FontSize == 0 && t.ContentWidth == 0 && t.Radius < 0 {
		return ""
	}
	if t.Background != "" {
		rules = append(rules, "body { background-color: var(--pr-bg); }")
	}
	if t.Text != "" {
		rules = append(rules, "body { color: var(--pr-text); }")
	}
	if t.BodyFont != "" {
		rules = append(rules, "body { font-family: var(--pr-body-font); }")
	}
	if t.FontSize != 0 {
		rules = append(rules, fmt.Sprintf("html { font-size: %dpx; }", t.FontSize))
	}
	if t.HeadingFont != "" {
		rules = append(rules, "h1, h2, h3, h4, h5, h6 { font-family: var(--pr-heading-font); }")
	}
	if t.Heading != "" {
		rules = append(rules, "h1, h2, h3, h4, h5, h6 { color: var(--pr-heading); }")
	}
	if t.Primary != "" {
		rules = append(rules,
			"a, a:visited { color: var(--pr-primary); }",
			"button, .button, .btn, input[type=submit] { background-color: var(--pr-primary); border-color: var(--pr-primary); }")
	}
	if t.Radius >= 0 {
		rules = append(rules, fmt.Sprintf("button, .button, .btn, input, textarea, select, img { border-radius: %dpx; }", t.Radius))
	}
	if t.ContentWidth != 0 {
		rules = append(rules, fmt.Sprintf("main, article { max-width: %dpx; margin-left: auto; margin-right: auto; }", t.ContentWidth))
	}
	var b strings.Builder
	b.WriteString(t.fontFaces())
	if len(vars) > 0 {
		b.WriteString(":root {\n" + strings.Join(vars, "\n") + "\n}\n")
	}
	b.WriteString(strings.Join(rules, "\n"))
	b.WriteString("\n")
	return b.String()
}

type Hook struct {
	Kind       string `json:"kind"`   // "custom-css" (theme loads assets/css/custom.css), "partial", or "none"
	Target     string `json:"target"` // file Pageroom writes
	Supported  bool   `json:"supported"`
	Suggestion string `json:"suggestion,omitempty"`
}

type Engine struct {
	Root     string
	Theme    string
	HeadHook string // override from .pageroom.yaml
}

const stylesheet = "assets/css/pageroom.css"

var partialCall = regexp.MustCompile(`partial(?:Cached)?\s+"([^"]+)"`)

func (e *Engine) DetectHook() Hook {
	if e.HeadHook != "" {
		return Hook{Kind: "partial", Target: e.partialPath(e.HeadHook), Supported: true}
	}
	dirs := []string{filepath.Join(e.Root, "layouts")}
	if e.Theme != "" {
		dirs = append(dirs, filepath.Join(e.Root, "themes", e.Theme, "layouts"))
	}
	var calls []string
	customCSS := false
	for _, d := range dirs {
		_ = filepath.WalkDir(d, func(p string, de fs.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.HasSuffix(p, ".html") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			if bytes.Contains(b, []byte("css/custom.css")) {
				customCSS = true
			}
			for _, m := range partialCall.FindAllSubmatch(b, -1) {
				calls = append(calls, string(m[1]))
			}
			return nil
		})
	}
	if customCSS {
		return Hook{Kind: "custom-css", Target: "assets/css/custom.css", Supported: true}
	}
	if name := bestHeadHook(calls); name != "" {
		return Hook{Kind: "partial", Target: e.partialPath(name), Supported: true}
	}
	return Hook{Kind: "none", Supported: false, Suggestion: `Your theme has no place for extra styles. Ask your developer to add {{ partial "pageroom.html" . }} inside <head> in the theme's base layout.`}
}

// bestHeadHook picks the partial a theme provides for site owners to extend <head>.
func bestHeadHook(calls []string) string {
	preferred := []string{"extend_head.html", "head/custom.html", "hooks/head-end.html", "custom_head.html",
		"custom-head.html", "head-custom.html", "docs/inject/head.html", "custom/head.html", "head/extend.html", "pageroom.html"}
	seen := map[string]bool{}
	for _, c := range calls {
		if !strings.HasSuffix(c, ".html") {
			c += ".html"
		}
		seen[c] = true
	}
	for _, p := range preferred {
		if seen[p] {
			return p
		}
	}
	for c := range seen {
		l := strings.ToLower(c)
		if strings.Contains(l, "head") && (strings.Contains(l, "custom") || strings.Contains(l, "extend") || strings.Contains(l, "inject") || strings.Contains(l, "hook")) {
			return c
		}
	}
	return ""
}

func (e *Engine) partialPath(name string) string {
	if !strings.HasSuffix(name, ".html") {
		name += ".html"
	}
	dir := "layouts/partials"
	// Hugo 0.146+ themes may use the new layouts/_partials folder.
	if e.Theme != "" && fsutil.Exists(filepath.Join(e.Root, "themes", e.Theme, "layouts", "_partials")) {
		dir = "layouts/_partials"
	}
	return dir + "/" + strings.TrimPrefix(name, "/")
}

const linkSnippet = `{{/* Pageroom styles */}}{{ with resources.Get "css/pageroom.css" }}<link rel="stylesheet" href="{{ (. | fingerprint).RelPermalink }}">{{ end }}`

func (e *Engine) Load() Style {
	var s Style
	s.Tokens.Radius = -1
	b, err := os.ReadFile(e.path(stylesheet))
	if err != nil {
		hook := e.DetectHook()
		if hook.Kind == "custom-css" {
			if b2, err := os.ReadFile(e.path(hook.Target)); err == nil {
				b = extractBlock(b2)
			}
		}
	}
	text := string(b)
	if i := strings.Index(text, tokensPfx); i >= 0 {
		rest := text[i+len(tokensPfx):]
		if j := strings.Index(rest, " */"); j >= 0 {
			_ = json.Unmarshal([]byte(rest[:j]), &s.Tokens)
		}
	}
	if i := strings.Index(text, customMark); i >= 0 {
		s.CustomCSS = strings.TrimSpace(text[i+len(customMark):])
	}
	return s
}

func extractBlock(b []byte) []byte {
	s := string(b)
	i := strings.Index(s, beginMarker)
	j := strings.Index(s, endMarker)
	if i < 0 || j < i {
		return nil
	}
	return []byte(s[i+len(beginMarker) : j])
}

func (e *Engine) path(rel string) string { return filepath.Join(e.Root, filepath.FromSlash(rel)) }

func (e *Engine) render(s Style) string {
	tok, _ := json.Marshal(s.Tokens)
	var b strings.Builder
	b.WriteString(tokensPfx + string(tok) + " */\n")
	b.WriteString(s.Tokens.CSS())
	b.WriteString("\n" + customMark + "\n")
	b.WriteString(strings.TrimSpace(s.CustomCSS) + "\n")
	return b.String()
}

// Save writes the stylesheet and makes sure the theme loads it. It returns
// the hook that was used.
func (e *Engine) Save(s Style) (Hook, error) {
	if err := s.Tokens.Validate(); err != nil {
		return Hook{}, err
	}
	if strings.Contains(s.CustomCSS, "</style") || strings.Contains(strings.ToLower(s.CustomCSS), "<script") {
		return Hook{}, errors.New("custom CSS can't contain HTML tags")
	}
	css := e.render(s)
	hook := e.DetectHook()
	switch hook.Kind {
	case "custom-css":
		// The theme already loads custom.css: put our block inside it,
		// leaving anything else the developer wrote there alone.
		existing, _ := os.ReadFile(e.path(hook.Target))
		return hook, fsutil.WriteFileAtomic(e.path(hook.Target), replaceBlock(existing, css), 0o644)
	case "partial":
		if err := fsutil.WriteFileAtomic(e.path(stylesheet), []byte(css), 0o644); err != nil {
			return hook, err
		}
		return hook, e.ensureSnippet(hook.Target)
	default:
		// Still save the file so nothing is lost. It starts working as
		// soon as the theme includes it.
		return hook, fsutil.WriteFileAtomic(e.path(stylesheet), []byte(css), 0o644)
	}
}

func replaceBlock(existing []byte, css string) []byte {
	block := beginMarker + "\n" + css + endMarker + "\n"
	s := string(existing)
	i := strings.Index(s, beginMarker)
	j := strings.Index(s, endMarker)
	if i >= 0 && j > i {
		end := j + len(endMarker)
		if end < len(s) && s[end] == '\n' {
			end++
		}
		return []byte(s[:i] + block + s[end:])
	}
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return []byte(s + block)
}

// ensureSnippet adds our <link> to the site's override of the theme's head
// partial. Content the theme had in that partial is copied first.
func (e *Engine) ensureSnippet(target string) error {
	full := e.path(target)
	existing, err := os.ReadFile(full)
	if err != nil && e.Theme != "" {
		themeFile := filepath.Join(e.Root, "themes", e.Theme, filepath.FromSlash(target))
		existing, _ = os.ReadFile(themeFile)
	}
	if bytes.Contains(existing, []byte(`resources.Get "css/pageroom.css"`)) {
		return nil
	}
	out := append(existing, []byte("\n"+linkSnippet+"\n")...)
	if len(existing) == 0 {
		out = []byte(linkSnippet + "\n")
	}
	return fsutil.WriteFileAtomic(full, out, 0o644)
}
