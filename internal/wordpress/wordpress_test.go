package wordpress

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/myrheimb/pageroom/internal/appearance"
	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/starter"
)

type fakeFetcher struct{ got []string }

func (f *fakeFetcher) Get(_ context.Context, u string) ([]byte, error) {
	f.got = append(f.got, u)
	if strings.HasSuffix(u, "/menu.pdf") || strings.HasSuffix(u, "/oven.mp3") {
		return []byte("file"), nil
	}
	if !strings.HasSuffix(u, "/loaf.jpg") && !strings.HasSuffix(u, "/cropped-icon.png") {
		return nil, errors.New("404")
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
	return b.Bytes(), nil
}

func load(t *testing.T) *Export {
	f, err := os.Open("testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	e, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestSummarize(t *testing.T) {
	s := load(t).Summarize()
	if s.Title != "Hannah's Bakery" || s.Posts != 3 || s.Pages != 2 || s.Drafts != 1 || s.Images != 3 || s.MenuItems != 3 || s.Comments != 1 ||
		!slices.Equal(s.Skipped, []SkippedKind{{Type: "product", Count: 1}}) {
		t.Fatalf("%+v", s)
	}
}

func TestImport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "site")
	if err := starter.CreateEmpty(root, "Hannah's Bakery", "0.168.0"); err != nil {
		t.Fatal(err)
	}
	s, err := site.Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ff := &fakeFetcher{}
	rep, err := Import(context.Background(), load(t), Options{Site: s, DownloadImages: true, Fetch: ff})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Posts != 3 || rep.Pages != 2 || rep.Drafts != 1 || rep.MenuItems != 3 || rep.Files != 2 {
		t.Fatalf("report %+v", rep)
	}
	if rep.Icon == nil || len(rep.FailedImages) != 0 {
		t.Errorf("the newest Site Icon wasn't downloaded: %v", rep.FailedImages)
	}
	for _, u := range ff.got {
		if strings.Contains(u, "evil.internal") {
			t.Fatal("downloaded from a host that isn't the WordPress site")
		}
	}

	post, err := os.ReadFile(filepath.Join(root, "content/posts/our-new-sourdough/index.md"))
	if err != nil {
		t.Fatal(err)
	}
	p := string(post)
	for _, want := range []string{
		"title = 'Our new sourdough'", "date = 2024-03-02T09:30:00Z", "categories = ['Bread', 'Coffee & cake']", "tags = ['sourdough']",
		"aliases = ['/2024/03/02/our-new-sourdough/']", "description = \"The loaf we're proudest of.\"", "author = 'Hannah Baker'",
		"image = 'loaf.jpg'", "We spent **months** perfecting", "[story](/about/)", "![A sourdough loaf](loaf.jpg)",
		"{{< youtube dQw4w9WgXcQ >}}", "- Flour", "{{< contact-form >}}", "https://evil.internal/x.jpg",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("post missing %q:\n%s", want, p)
		}
	}
	if strings.Contains(p, "wp:paragraph") || strings.Contains(p, "draft") {
		t.Errorf("leftover WordPress markup or wrong draft flag:\n%s", p)
	}
	if _, err := os.Stat(filepath.Join(root, "content/posts/our-new-sourdough/loaf.jpg")); err != nil {
		t.Error("image not downloaded into the post's folder")
	}
	if len(rep.Warnings) != 3 || !strings.Contains(rep.Warnings[0], "your-phone") || !strings.Contains(rep.Warnings[1], "forms.example.com") ||
		!strings.Contains(rep.Warnings[2], "“About”") {
		t.Errorf("warnings %v", rep.Warnings)
	}
	if !slices.Equal(rep.ContactForms, []string{"Our new sourdough", "About"}) || rep.ContactEmail != "hannah@hannahsbakery.example" {
		t.Errorf("contact forms %v, email %q", rep.ContactForms, rep.ContactEmail)
	}
	if _, err := os.Stat(filepath.Join(root, "layouts/shortcodes/contact-form.html")); err != nil {
		t.Error("the contact form shortcode wasn't added")
	}

	classic, _ := os.ReadFile(filepath.Join(root, "content/posts/classic-post/index.md"))
	c := string(classic)
	for _, want := range []string{"draft = true", "First paragraph with a line", "<!--more-->", "Second paragraph.", "Our loaf", "](loaf.jpg)"} {
		if !strings.Contains(c, want) {
			t.Errorf("classic post missing %q:\n%s", want, c)
		}
	}

	baking, _ := os.ReadFile(filepath.Join(root, "content/posts/baking-day/index.md"))
	bakingPage := string(baking)
	for _, want := range []string{
		"Rye[^1] is in the oven.", "\n<!--more-->\n", "[menu](menu.pdf)", `{{< audio src="oven.mp3" >}}`,
		`{{< video src="https://hannahsbakery.example/wp-content/uploads/2024/04/knead.mp4" >}}`,
		`{{< google-map src="https://www.google.com/maps/embed?pb=!1m18!1m12" title="Hannah's Bakery on the map" >}}`,
		"{{< youtube dQw4w9WgXcQ >}}", "{{< gist hannah 0123abcd >}}", "[^1]: Mostly _whole_ rye.",
	} {
		if !strings.Contains(bakingPage, want) {
			t.Errorf("baking day missing %q:\n%s", want, bakingPage)
		}
	}
	if strings.Contains(bakingPage, "forms.example.com") || strings.Contains(bakingPage, "abc-123") || strings.Contains(bakingPage, "tracking") {
		t.Errorf("leftover embed or footnote link:\n%s", bakingPage)
	}
	for _, name := range []string{"content/posts/baking-day/menu.pdf", "content/posts/baking-day/oven.mp3", "layouts/shortcodes/audio.html",
		"layouts/shortcodes/gist.html", "layouts/shortcodes/google-map.html", "layouts/shortcodes/video.html"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("%s wasn't written", name)
		}
	}
	if !slices.Equal(rep.FailedFiles, []string{"https://hannahsbakery.example/wp-content/uploads/2024/04/knead.mp4"}) {
		t.Errorf("failed files %v", rep.FailedFiles)
	}

	// A page with children becomes a section. Its child nests under it.
	about, err := os.ReadFile(filepath.Join(root, "content/about/_index.md"))
	if err != nil {
		t.Error("parent page should be content/about/_index.md")
	}
	if aboutPage := string(about); !strings.Contains(aboutPage, "Open daily from 7.") || strings.Count(aboutPage, "Closed on Mondays.") != 1 ||
		!strings.Contains(aboutPage, "{{< contact-form >}}") || strings.Contains(aboutPage, "wp:block") {
		t.Errorf("reusable blocks not put in once each:\n%s", aboutPage)
	}
	if _, err := os.Stat(filepath.Join(root, "content/about/team/index.md")); err != nil {
		t.Error("child page should be content/about/team/index.md")
	}

	_ = s.Reload()
	menus, _ := s.Menus()
	var main *site.Menu
	for i := range menus {
		if menus[i].Name == "main" {
			main = &menus[i]
		}
	}
	if main == nil || len(main.Items) != 3 || main.Items[0].PageRef != "/about" || main.Items[1].URL != "https://instagram.com/hannahsbakery" ||
		main.Items[2].PageRef != "/categories/Coffee & cake" || main.Items[2].Name != "Coffee & cake" {
		t.Fatalf("menu %+v", main)
	}

	// The imported site must build with Hugo (when Hugo is installed).
	if bin, err := exec.LookPath("hugo"); err == nil {
		out, err := exec.Command(bin, "--source", root, "--renderToMemory", "--noBuildLock", "--quiet").CombinedOutput()
		if err != nil {
			t.Fatalf("imported site doesn't build: %v\n%s", err, out)
		}
	}

	// Importing twice never overwrites. Without downloads, images keep
	// pointing at the old site.
	rep, err = Import(context.Background(), load(t), Options{Site: s, DownloadImages: false, Fetch: ff})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Icon != nil {
		t.Error("downloaded the Site Icon with downloads off")
	}
	again, _ := os.ReadFile(filepath.Join(root, "content/posts/our-new-sourdough-2/index.md"))
	if !strings.Contains(string(again), "](https://hannahsbakery.example/wp-content/uploads/2024/03/loaf-1024x683.jpg)") || !strings.Contains(string(again), "[story](/about/)") {
		t.Errorf("undownloaded image or page link wrong:\n%s", again)
	}
	if _, err := os.Stat(filepath.Join(root, "content/posts/our-new-sourdough-2/index.md")); err != nil {
		t.Error("second import should go to a new folder")
	}
}

func TestCustomCSSComments(t *testing.T) {
	got := customCSSComments(load(t))
	for _, want := range []string{
		"/* Additional CSS from the WordPress theme twentytwentyfour, switched off:",
		".hero { color: red; } /* old note * / .cta { margin: 0; }",
		"/* Additional CSS from the WordPress theme oceanwp, switched off:",
		".menu { display: none; }",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Count(got, "*/") != 2 {
		t.Errorf("an old */ ended a comment early:\n%s", got)
	}
	if got := customCSSComments(&Export{}); got != "" {
		t.Errorf("no custom_css should give nothing, got %q", got)
	}
	if got := commentSafe("</style><SCRIPT>"); strings.Contains(strings.ToLower(got), "</style") || strings.Contains(strings.ToLower(got), "<script") {
		t.Errorf("HTML tags survive: %q", got)
	}
}

func TestParseRejectsNonWXR(t *testing.T) {
	if _, err := Parse(strings.NewReader("<html><body>nope</body></html>")); err == nil {
		t.Fatal("accepted HTML")
	}
}

func TestStylesRadius(t *testing.T) {
	page := func(radius string) []byte {
		return []byte(`<html><head><style id='global-styles-inline-css'>:root{--wp--preset--color--accent: #345C00;}` +
			`.wp-element-button{background-color: var(--wp--preset--color--accent);border-radius:` + radius + `}</style></head></html>`)
	}
	for radius, want := range map[string]int{"0": 0, "6px": 6, "40px": appearance.MaxRadius, "9999px": -1} {
		got := stylesOf(page(radius))
		if got == nil || got.Radius != want {
			t.Errorf("radius %s: got %+v, want %d", radius, got, want)
		}
	}
}

func TestStyles(t *testing.T) {
	page := func(css string) []byte {
		return []byte(`<html><head><style id='global-styles-inline-css'>` + css + `</style></head></html>`)
	}
	presets := `:root{--wp--preset--color--base: #F9F9F9;--wp--preset--color--contrast: #111111;--wp--preset--color--accent: #345C00;` +
		`--wp--preset--font-family--body: "Inter", sans-serif;--wp--preset--font-family--heading: "Source Serif Pro", serif;}` +
		`:root { --wp--style--global--content-size: 620px;--wp--style--global--wide-size: 1280px; }`
	body := `body{background-color: var(--wp--preset--color--base);color: var(--wp--preset--color--contrast);font-family: var(--wp--preset--font-family--body);}` +
		`h1, h2, h3, h4, h5, h6{color: var(--wp--preset--color--contrast);font-family: var(--wp--preset--font-family--heading);}`
	want := appearance.Tokens{Primary: "#345c00", Background: "#f9f9f9", Text: "#111111", Heading: "#111111", BodyFont: "system", HeadingFont: "transitional", ContentWidth: 620, Radius: -1}
	tests := []struct {
		name string
		page []byte
		want *appearance.Tokens
	}{
		{"block theme", page(presets + body + `a:where(:not(.wp-element-button)){color: var(--wp--preset--color--accent);}`), &want},
		{"links follow the text, buttons carry the accent", page(presets + body + `a:where(:not(.wp-element-button)){color: currentColor;}` +
			`:root :where(.wp-element-button, .wp-block-button__link){background-color: var(--wp--preset--color--accent);}`), &want},
		{"WordPress before 6.6", page(presets + body + `.wp-element-button, .wp-block-button__link{background-color: var(--wp--preset--color--accent);}`), &want},
		{"classic theme", []byte(`<html><head><link rel="stylesheet" href="style.css"></head></html>`), nil},
		{"global styles without a design", page(`:root{--wp--preset--color--black: #000000;}body .is-layout-flex{display: flex;}`), nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := stylesOf(test.page)
			if (got == nil) != (test.want == nil) || got != nil && !reflect.DeepEqual(*got, *test.want) {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestStylesOfRealThemes(t *testing.T) {
	tests := []struct {
		theme string
		want  appearance.Tokens
	}{
		{"twentytwentyfive", appearance.Tokens{Primary: "#111111", Background: "#ffffff", Text: "#111111", BodyFont: "system", ContentWidth: 645, Radius: -1}},
		{"astra", appearance.Tokens{Primary: "#060097", Background: "#f9f6fe", Text: "#67768e", Heading: "#1e293b", BodyFont: "system", HeadingFont: "system", FontSize: 18, ContentWidth: 1200, Radius: 4}},
		{"generatepress", appearance.Tokens{Primary: "#1e73be", Background: "#f7f8f9", Text: "#222222", BodyFont: "system", FontSize: 17, Radius: -1}},
		{"oceanwp", appearance.Tokens{Primary: "#13aff0", Background: "#ffffff", Text: "#4a4a4a", Heading: "#333333", BodyFont: "system", FontSize: 14, Radius: -1}},
		{"kadence", appearance.Tokens{Primary: "#2b6cb0", Background: "#f7fafc", Text: "#2d3748", Heading: "#1a202c", BodyFont: "system", FontSize: 17, Radius: 3}},
	}
	for _, test := range tests {
		t.Run(test.theme, func(t *testing.T) {
			page, err := os.ReadFile(filepath.Join("testdata", "themes", test.theme+".html"))
			if err != nil {
				t.Fatal(err)
			}

			var sheets [][]byte
			if sheet, err := os.ReadFile(filepath.Join("testdata", "themes", test.theme+"-style.css")); err == nil {
				sheets = append(sheets, sheet)
			}

			got := stylesOf(page, sheets...)
			if got == nil || !reflect.DeepEqual(*got, test.want) {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
		})
	}
}

type homepageFetcher struct{ files map[string]string }

func (f homepageFetcher) Get(_ context.Context, rawURL string) ([]byte, error) {
	body, ok := f.files[rawURL]
	if !ok {
		return nil, errors.New("404")
	}
	return []byte(body), nil
}

func TestImportReadsLinkedStylesheets(t *testing.T) {
	oceanwp, err := os.ReadFile(filepath.Join("testdata", "themes", "oceanwp.html"))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := os.ReadFile(filepath.Join("testdata", "themes", "oceanwp-style.css"))
	if err != nil {
		t.Fatal(err)
	}
	styleUrl := "https://wp-themes.com/wp-content/themes/oceanwp/assets/css/style.min.css?ver=4.2.8"
	fontUrl := "https://wp-themes.com/wp-content/themes/oceanwp/assets/fonts/fontawesome/css/all.min.css?ver=6.7.2"
	homepage := string(oceanwp)

	tests := []struct {
		name     string
		homepage string
		files    map[string]string
		want     appearance.Tokens
	}{
		{"downloaded", homepage, map[string]string{styleUrl: string(stylesheet), fontUrl: "body{color:#ff0000}"},
			appearance.Tokens{Primary: "#13aff0", Background: "#ffffff", Text: "#4a4a4a", Heading: "#333333", BodyFont: "system", FontSize: 14, Radius: -1}},
		{"failed download keeps the inline results", homepage + "<style>body{color:#123456}</style>", map[string]string{},
			appearance.Tokens{Text: "#123456", FontSize: 14, Radius: -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "site")
			if err := starter.CreateEmpty(root, "Ocean", "0.168.0"); err != nil {
				t.Fatal(err)
			}
			s, err := site.Open(root, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			test.files["https://wp-themes.com/"] = test.homepage
			export := &Export{Title: "Ocean", Link: "https://wp-themes.com/"}

			rep, err := Import(context.Background(), export, Options{Site: s, DownloadImages: true, Fetch: homepageFetcher{test.files}})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Style == nil || !reflect.DeepEqual(*rep.Style, test.want) {
				t.Fatalf("got %+v, want %+v", rep.Style, test.want)
			}
		})
	}
}

func TestImportCopiesSiteFonts(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("testdata", "themes", "twentytwentyfive.html"))
	if err != nil {
		t.Fatal(err)
	}
	fontUrl := "https://wp-themes.com/wp-content/themes/twentytwentyfive/assets/fonts/manrope/Manrope-VariableFont_wght.woff2"
	fontPath := "fonts/manrope-variablefont-wght.woff2"

	tests := []struct {
		name  string
		files map[string]string
		want  appearance.Tokens
	}{
		{"downloaded", map[string]string{fontUrl: "woff2 data"}, appearance.Tokens{
			Primary: "#111111", Background: "#ffffff", Text: "#111111", BodyFont: "Manrope", HeadingFont: "Manrope", ContentWidth: 645, Radius: -1,
			Fonts: []appearance.SiteFont{{Family: "Manrope", Fallback: "system", Files: []appearance.FontFile{{Path: fontPath, Weight: "200 800", Style: "normal"}}}},
		}},
		{"failed download keeps the built-in stack", map[string]string{}, appearance.Tokens{
			Primary: "#111111", Background: "#ffffff", Text: "#111111", BodyFont: "system", ContentWidth: 645, Radius: -1,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "site")
			if err := starter.CreateEmpty(root, "Fonts", "0.168.0"); err != nil {
				t.Fatal(err)
			}
			s, err := site.Open(root, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			test.files["https://wp-themes.com/"] = string(page)

			rep, err := Import(context.Background(), &Export{Title: "Fonts", Link: "https://wp-themes.com/"}, Options{Site: s, DownloadImages: true, Fetch: homepageFetcher{test.files}})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Style == nil || !reflect.DeepEqual(*rep.Style, test.want) {
				t.Fatalf("got %+v, want %+v", rep.Style, test.want)
			}

			data, err := os.ReadFile(filepath.Join(root, "static", fontPath))
			if len(test.want.Fonts) > 0 && (err != nil || string(data) != "woff2 data") {
				t.Fatalf("font file: %q, %v", data, err)
			}
			if len(test.want.Fonts) == 0 && err == nil {
				t.Fatal("wrote a font file that wasn't downloaded")
			}
		})
	}
}

func TestImportCopiesGoogleFonts(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("testdata", "themes", "astra.html"))
	if err != nil {
		t.Fatal(err)
	}
	googleUrl := "https://fonts.googleapis.com/css?family=Inter%3A400%7CPlus+Jakarta+Sans%3A600&display=fallback"
	googleCss := `/* latin */
@font-face {
  font-family: 'Inter';
  font-style: normal;
  font-weight: 400;
  src: url(https://fonts.gstatic.com/s/inter/v13/inter-latin.woff2) format('woff2');
  unicode-range: U+0000-00FF, U+0131;
}
@font-face {
  font-family: 'Plus Jakarta Sans';
  font-style: normal;
  font-weight: 600;
  src: url(https://fonts.gstatic.com/s/plusjakartasans/v8/jakarta-latin.woff2) format('woff2');
}`
	adobePage := strings.Replace(string(page), "<head>", `<head><link rel="stylesheet" href="https://use.typekit.net/abc1234.css">`, 1)
	adobeCss := `@font-face{font-family:"Inter";src:url("https://use.typekit.net/af/inter.woff2") format("woff2");font-weight:400}`

	tests := []struct {
		name  string
		page  string
		files map[string]string
		want  appearance.Tokens
	}{
		{"downloaded", string(page), map[string]string{
			googleUrl: googleCss,
			"https://fonts.gstatic.com/s/inter/v13/inter-latin.woff2":            "inter data",
			"https://fonts.gstatic.com/s/plusjakartasans/v8/jakarta-latin.woff2": "jakarta data",
		}, appearance.Tokens{
			Primary: "#060097", Background: "#f9f6fe", Text: "#67768e", Heading: "#1e293b", BodyFont: "Inter", HeadingFont: "Plus Jakarta Sans", FontSize: 18, ContentWidth: 1200, Radius: 4,
			Fonts: []appearance.SiteFont{
				{Family: "Inter", Fallback: "system", Files: []appearance.FontFile{{Path: "fonts/inter-latin.woff2", Weight: "400", Style: "normal", UnicodeRange: "U+0000-00FF, U+0131"}}},
				{Family: "Plus Jakarta Sans", Fallback: "system", Files: []appearance.FontFile{{Path: "fonts/jakarta-latin.woff2", Weight: "600", Style: "normal"}}},
			},
		}},
		{"other font services keep the built-in stack", adobePage, map[string]string{
			"https://use.typekit.net/abc1234.css":    adobeCss,
			"https://use.typekit.net/af/inter.woff2": "adobe data",
		}, appearance.Tokens{
			Primary: "#060097", Background: "#f9f6fe", Text: "#67768e", Heading: "#1e293b", BodyFont: "system", HeadingFont: "system", FontSize: 18, ContentWidth: 1200, Radius: 4,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "site")
			if err := starter.CreateEmpty(root, "Fonts", "0.168.0"); err != nil {
				t.Fatal(err)
			}
			s, err := site.Open(root, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			test.files["https://wp-themes.com/"] = test.page

			rep, err := Import(context.Background(), &Export{Title: "Fonts", Link: "https://wp-themes.com/"}, Options{Site: s, DownloadImages: true, Fetch: homepageFetcher{test.files}})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Style == nil || !reflect.DeepEqual(*rep.Style, test.want) {
				t.Fatalf("got %+v, want %+v", rep.Style, test.want)
			}
			for _, font := range test.want.Fonts {
				if _, err := os.Stat(filepath.Join(root, "static", font.Files[0].Path)); err != nil {
					t.Fatal(err)
				}
			}
			if entries, _ := os.ReadDir(filepath.Join(root, "static", "fonts")); len(test.want.Fonts) == 0 && len(entries) > 0 {
				t.Fatal("copied a font from a service that forbids it")
			}
		})
	}
}

func TestImportLogo(t *testing.T) {
	var logo bytes.Buffer
	_ = jpeg.Encode(&logo, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
	customLogoPage := `<a class="custom-logo-link"><img width="8" class="custom-logo" src="https://example.com/wp-content/uploads/logo-150x150.jpg"></a>`
	blockLogoPage := `<div class="wp-block-site-logo"><a><img src="/wp-content/uploads/logo.jpg"></a></div>`
	elsewhere := `<img class="custom-logo" src="https://cdn.other.com/logo.jpg">`

	tests := []struct {
		name     string
		homepage string
		existing string
		want     string
	}{
		{"custom logo", customLogoPage, "", "/logo.jpg"},
		{"site logo block", blockLogoPage, "", "/logo.jpg"},
		{"existing logo is left alone", customLogoPage, "/mine.png", "/mine.png"},
		{"logo from another host is skipped", elsewhere, "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "site")
			if err := starter.CreateEmpty(root, "Logo", "0.168.0"); err != nil {
				t.Fatal(err)
			}
			s, err := site.Open(root, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if test.existing != "" {
				if err := s.SetLogo(test.existing); err != nil {
					t.Fatal(err)
				}
			}
			fetcher := homepageFetcher{map[string]string{
				"https://example.com/":                                    test.homepage,
				"https://example.com/wp-content/uploads/logo.jpg":         logo.String(),
				"https://example.com/wp-content/uploads/logo-150x150.jpg": logo.String(),
			}}

			_, err = Import(context.Background(), &Export{Title: "Logo", Link: "https://example.com/"}, Options{Site: s, DownloadImages: true, Fetch: fetcher})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Reload(); err != nil {
				t.Fatal(err)
			}
			if got := s.Logo(); got != test.want {
				t.Fatalf("params.logo is %q, want %q", got, test.want)
			}
			if test.want == "/logo.jpg" {
				if _, err := os.Stat(filepath.Join(root, "static", "logo.jpg")); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// stylesOf reads the colors, fonts and content width from the inline styles
// of a homepage and from its linked stylesheets. WordPress's global styles
// go first, being the core defaults and the block theme's design, then the
// stylesheets, then the inline blocks, so that a classic theme's own rules
// win. It returns nil when the page has no design to read.
func stylesOf(page []byte, stylesheets ...[]byte) *appearance.Tokens {
	tokens, _ := readStyles(page, stylesheets...)
	return tokens
}
