package appearance

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func site(t *testing.T, files map[string]string) string {
	root := t.TempDir()
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(c), 0o644)
	}
	return root
}

func TestEmptyTokensChangeNothing(t *testing.T) {
	if css := (Tokens{Radius: -1}).CSS(); css != "" {
		t.Fatalf("got %q", css)
	}
}

func TestRadius(t *testing.T) {
	for radius, valid := range map[int]bool{-2: false, -1: true, 0: true, MaxRadius: true, MaxRadius + 1: false} {
		if got := (Tokens{Radius: radius}).Validate() == nil; got != valid {
			t.Errorf("radius %d: valid = %v, want %v", radius, got, valid)
		}
	}
	if css := (Tokens{Radius: 0}).CSS(); !strings.Contains(css, "border-radius: 0px;") {
		t.Fatalf("square corners missing: %q", css)
	}
	if css := (Tokens{Radius: 6}).CSS(); !strings.Contains(css, "border-radius: 6px;") {
		t.Fatalf("got %q", css)
	}
}

func TestSavedStyleWithoutRadiusIsThemeDefault(t *testing.T) {
	root := site(t, map[string]string{"assets/css/pageroom.css": tokensPfx + `{"primary":"#ff6600"} */` + "\n"})
	got := (&Engine{Root: root}).Load()
	if got.Tokens.Primary != "#ff6600" || got.Tokens.Radius != -1 {
		t.Fatalf("got %+v", got.Tokens)
	}
}

func TestValidateBlocksInjection(t *testing.T) {
	if err := (Tokens{Primary: "red;} body{display:none"}).Validate(); err == nil {
		t.Fatal("accepted CSS injection via color")
	}
	if err := (Tokens{BodyFont: "Comic Sans"}).Validate(); err == nil {
		t.Fatal("accepted unknown font")
	}
}

func TestPartialHookRoundTrip(t *testing.T) {
	root := site(t, map[string]string{
		"themes/paper/layouts/_default/baseof.html":      `<head>{{ partial "extend_head.html" . }}</head>`,
		"themes/paper/layouts/partials/extend_head.html": `<!-- theme extension point -->`,
	})
	e := &Engine{Root: root, Theme: "paper"}
	hook := e.DetectHook()
	if hook.Kind != "partial" || hook.Target != "layouts/partials/extend_head.html" {
		t.Fatalf("%+v", hook)
	}
	in := Style{Tokens: Tokens{Primary: "#ff6600", BodyFont: "humanist", Radius: 6}, CustomCSS: ".hero { padding: 2rem; }"}
	if _, err := e.Save(in); err != nil {
		t.Fatal(err)
	}
	override, _ := os.ReadFile(filepath.Join(root, "layouts/partials/extend_head.html"))
	if !strings.Contains(string(override), "theme extension point") || !strings.Contains(string(override), "pageroom.css") {
		t.Fatalf("override: %s", override)
	}
	e.Save(in) // idempotent
	override2, _ := os.ReadFile(filepath.Join(root, "layouts/partials/extend_head.html"))
	if strings.Count(string(override2), "pageroom.css") != 1 {
		t.Fatal("snippet added twice")
	}
	if _, err := os.Stat(filepath.Join(root, "themes/paper/layouts/partials/extend_head.html")); err != nil {
		t.Fatal("theme file touched")
	}
	got := e.Load()
	if !reflect.DeepEqual(got.Tokens, in.Tokens) || got.CustomCSS != in.CustomCSS {
		t.Fatalf("load: %+v", got)
	}
}

func TestCustomCSSHookPreservesDeveloperCSS(t *testing.T) {
	root := site(t, map[string]string{
		"themes/blow/layouts/partials/head.html": `{{ $c := resources.Get "css/custom.css" }}`,
		"assets/css/custom.css":                  ".dev { color: red; }\n",
	})
	e := &Engine{Root: root, Theme: "blow"}
	in := Style{Tokens: Tokens{Background: "#ffffff", Radius: -1}}
	if _, err := e.Save(in); err != nil {
		t.Fatal(err)
	}
	in.Tokens.Background = "#000000"
	e.Save(in)
	b, _ := os.ReadFile(filepath.Join(root, "assets/css/custom.css"))
	s := string(b)
	if !strings.Contains(s, ".dev { color: red; }") || strings.Count(s, beginMarker) != 1 || !strings.Contains(s, "#000000") {
		t.Fatalf("custom.css:\n%s", s)
	}
	if got := e.Load(); got.Tokens.Background != "#000000" {
		t.Fatalf("load %+v", got)
	}
}

func TestNoHook(t *testing.T) {
	root := site(t, map[string]string{"themes/bare/layouts/_default/baseof.html": `<head></head>`})
	e := &Engine{Root: root, Theme: "bare"}
	if h := e.DetectHook(); h.Supported || h.Suggestion == "" {
		t.Fatalf("%+v", h)
	}
}

var manrope = SiteFont{Family: "Manrope", Fallback: "geometric", Files: []FontFile{
	{Path: "fonts/manrope.woff2", Weight: "200 800", Style: "normal", UnicodeRange: "U+0000-00FF, U+0131"},
}}

func TestSiteFontCSS(t *testing.T) {
	tokens := Tokens{BodyFont: "Manrope", HeadingFont: "Manrope", Fonts: []SiteFont{manrope}, Radius: -1}
	if err := tokens.Validate(); err != nil {
		t.Fatal(err)
	}
	css := tokens.CSS()
	for _, want := range []string{
		`@font-face {`,
		`font-family: "Manrope";`,
		`src: url("../fonts/manrope.woff2") format("woff2");`,
		`font-weight: 200 800;`,
		`font-style: normal;`,
		`unicode-range: U+0000-00FF, U+0131;`,
		`--pr-body-font: "Manrope", ` + FontStacks["geometric"] + `;`,
		`--pr-heading-font: "Manrope", ` + FontStacks["geometric"] + `;`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q in:\n%s", want, css)
		}
	}
	if strings.Contains(css, "url(\"/") || strings.Contains(css, "http") {
		t.Errorf("font URL is not relative:\n%s", css)
	}
}

func TestSiteFontUnusedOrUnlisted(t *testing.T) {
	if err := (Tokens{BodyFont: "Manrope"}).Validate(); err == nil {
		t.Fatal("accepted a font that is not listed")
	}
	if css := (Tokens{BodyFont: "humanist", Fonts: []SiteFont{manrope}, Radius: -1}).CSS(); strings.Contains(css, "@font-face") {
		t.Fatalf("unused font emitted:\n%s", css)
	}
}

func TestSiteFontValidationBlocksInjection(t *testing.T) {
	bad := manrope
	bad.Files = []FontFile{{Path: "fonts/a.woff2\") ;} body{display:none", Weight: "400", Style: "normal"}}
	if err := (Tokens{Fonts: []SiteFont{bad}}).Validate(); err == nil {
		t.Fatal("accepted a hostile font path")
	}
	bad = manrope
	bad.Family = `x"; } body{display:none`
	if err := (Tokens{Fonts: []SiteFont{bad}}).Validate(); err == nil {
		t.Fatal("accepted a hostile family")
	}
}

func TestSiteFontFormats(t *testing.T) {
	for extension, format := range map[string]string{"woff2": "woff2", "woff": "woff", "ttf": "truetype", "otf": "opentype"} {
		font := SiteFont{Family: "Manrope", Fallback: "geometric", Files: []FontFile{{Path: "fonts/manrope." + extension, Weight: "400", Style: "normal"}}}
		css := Tokens{BodyFont: "Manrope", Fonts: []SiteFont{font}, Radius: -1}.CSS()
		want := `src: url("../fonts/manrope.` + extension + `") format("` + format + `");`
		if !strings.Contains(css, want) {
			t.Errorf("missing %q in:\n%s", want, css)
		}
	}
}
