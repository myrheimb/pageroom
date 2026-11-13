package site_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/starter"
)

const blocksPage = `---
title: Blocks
---

Before.

{{< pr-hero image="beach.jpg" alt="A beach" button="Read more" link="/about/" >}}
## Welcome

A **bold** start.
{{< /pr-hero >}}

{{< pr-cards >}}
{{< pr-card >}}
### One

First _card_.
{{< /pr-card >}}

{{< pr-card >}}
### Two

- a
- b
{{< /pr-card >}}
{{< /pr-cards >}}

{{< pr-button link="/contact/" >}}Get in touch{{< /pr-button >}}

{{< pr-align to="center" >}}
Centered text.
{{< /pr-align >}}

{{< pr-button link="/right/" align="right" >}}Right{{< /pr-button >}}

{{< pr-hero align="center" >}}
Centered banner.
{{< /pr-hero >}}

{{< pr-cards align="center" >}}
{{< pr-card >}}
Centered card.
{{< /pr-card >}}
{{< /pr-cards >}}

{{< pr-columns >}}
{{< pr-column >}}
Left text.

{{< pr-button link="/one/" align="right" >}}First{{< /pr-button >}}

Between buttons.

{{< pr-button link="/two/" >}}Second{{< /pr-button >}}
{{< /pr-column >}}

{{< pr-column >}}
![Sea](sea.jpg)

~~~go
fmt.Println("hi")
~~~

{{< pr-video id="abc123" >}}
{{< /pr-column >}}
{{< /pr-columns >}}

{{< pr-columns layout="2-1" valign="bottom" >}}
{{< pr-column >}}
Wide first.

Wide second.
{{< /pr-column >}}

{{< pr-column >}}
Narrow.
{{< /pr-column >}}
{{< /pr-columns >}}

{{< pr-columns layout="1-1-1" valign="center" >}}
{{< pr-column >}}
One.
{{< /pr-column >}}

{{< pr-column >}}
Two.
{{< /pr-column >}}

{{< pr-column >}}
Three.
{{< /pr-column >}}
{{< /pr-columns >}}

After.
`

var starterThemes = []string{"pageroom-paper", "pageroom-soft", "pageroom-studio", "pageroom-tinted"}

func buildBlocksSite(t *testing.T, theme string, extraEnv ...string) string {
	t.Helper()
	return buildPageSite(t, theme, blocksPage, extraEnv...)
}

func buildPageSite(t *testing.T, theme, page string, extraEnv ...string) string {
	t.Helper()
	hugo, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("hugo is not installed")
	}
	dir := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(dir, "Blocks", "0.166.0"); err != nil {
		t.Fatal(err)
	}
	s, err := site.Open(dir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pr-align", "pr-button", "pr-card", "pr-cards", "pr-column", "pr-columns", "pr-hero", "pr-video"} {
		if err := s.AddShortcode(name); err != nil {
			t.Fatal(err)
		}
	}
	pageDir := filepath.Join(dir, "content", "landing")
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pageDir, "index.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(hugo, "--source", dir, "--destination", filepath.Join(dir, "public"), "--theme", theme, "--cacheDir", t.TempDir())
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hugo: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "Raw HTML omitted") {
		t.Fatalf("hugo dropped raw HTML:\n%s", out)
	}
	return filepath.Join(dir, "public", "landing", "index.html")
}

func TestBlockTemplatesBuildOnEveryStarterTheme(t *testing.T) {
	for _, theme := range starterThemes {
		t.Run(theme, func(t *testing.T) {
			raw, err := os.ReadFile(buildBlocksSite(t, theme))
			if err != nil {
				t.Fatal(err)
			}
			html := string(raw)
			for _, want := range []string{
				`<section class="pr-hero">`,
				`<h2 id="welcome">Welcome</h2>`,
				`<strong>bold</strong>`,
				`<a class="pr-button" href="/about/">Read more</a>`,
				`<img src="/beach.jpg" alt="A beach">`,
				`<div class="pr-cards">`,
				`<h3 id="one">One</h3>`,
				`<em>card</em>`,
				`<li>a</li>`,
				`<a class="pr-button" href="/contact/">Get in touch</a>`,
				`<div class="pr-align-center">`,
				`<div style="text-align: right"><a class="pr-button" href="/right/">Right</a></div>`,
				`<section class="pr-hero" style="text-align: center">`,
				`<div class="pr-cards" style="text-align: center">`,
				`minmax(min(100%, 14rem), 1fr)`,
				`var(--pr-primary`,
			} {
				if !strings.Contains(html, want) {
					t.Errorf("missing %q", want)
				}
			}
			if got := strings.Count(html, `<div class="pr-card">`); got != 3 {
				t.Errorf("%d cards, want 3", got)
			}
			columns := html[strings.Index(html, `<div class="pr-columns">`):]
			for _, want := range []string{
				`<div class="pr-column">`,
				`<p>Left text.</p>`,
				`<div style="text-align: right"><a class="pr-button" href="/one/">First</a></div>`,
				`<p>Between buttons.</p>`,
				`<a class="pr-button" href="/two/">Second</a>`,
				`<img src="sea.jpg" alt="Sea">`,
				`fmt`,
				`<div class="pr-columns pr-columns-2-1 pr-columns-bottom">`,
				`<div class="pr-columns pr-columns-1-1-1 pr-columns-center">`,
			} {
				if !strings.Contains(columns, want) {
					t.Errorf("columns miss %q", want)
				}
			}
			if got := strings.Count(columns, `<div class="pr-column">`); got != 7 {
				t.Errorf("%d columns, want 7", got)
			}
			if strings.Index(columns, "Left text.") > strings.Index(columns, "/one/") || strings.Index(columns, "/one/") > strings.Index(columns, "Between buttons.") || strings.Index(columns, "Between buttons.") > strings.Index(columns, "/two/") {
				t.Error("column content is out of order")
			}
			if strings.Contains(html, "<!--pr-button") || strings.Contains(html, "<!--/pr-button") {
				t.Error("a button marker is left in the page")
			}
			if !strings.Contains(html, ".pr-cards {") {
				t.Error("the cards style is missing")
			}
			if strings.Index(html, "Before.") > strings.Index(html, `<section class="pr-hero">`) || strings.Index(html, "After.") < strings.Index(html, `<a class="pr-button" href="/contact/">`) {
				t.Error("blocks are not between the paragraphs")
			}
		})
	}
}

func TestFirstButtonInAColumnKeepsItsStyle(t *testing.T) {
	const page = `---
title: Columns first
---

{{< pr-columns >}}
{{< pr-column >}}
Text.

{{< pr-button link="/one/" >}}First{{< /pr-button >}}
{{< /pr-column >}}

{{< pr-column >}}
Other.
{{< /pr-column >}}
{{< /pr-columns >}}
`
	for _, theme := range starterThemes {
		t.Run(theme, func(t *testing.T) {
			raw, err := os.ReadFile(buildPageSite(t, theme, page))
			if err != nil {
				t.Fatal(err)
			}
			html := string(raw)
			if !strings.Contains(html, ".pr-button {") {
				t.Error("the button style is missing")
			}
			if strings.Contains(html, "<!--pr-button") || strings.Contains(html, "<!--/pr-button") {
				t.Error("a button marker is left in the page")
			}
		})
	}
}

func TestVideoSurvivesInAColumnAndOnThePage(t *testing.T) {
	const page = `---
title: Videos
---

{{< pr-columns >}}
{{< pr-column >}}
Before.

{{< pr-video id="incolumn" >}}

After.
{{< /pr-column >}}

{{< pr-column >}}
Other.
{{< /pr-column >}}
{{< /pr-columns >}}

{{< pr-video id="onpage" >}}

{{< pr-video vimeo="76979871" >}}

{{< pr-video src="https://example.com/knead.mp4" >}}
`
	for _, theme := range starterThemes {
		t.Run(theme, func(t *testing.T) {
			raw, err := os.ReadFile(buildPageSite(t, theme, page))
			if err != nil {
				t.Fatal(err)
			}
			html := string(raw)
			for _, want := range []string{
				`src="https://www.youtube-nocookie.com/embed/incolumn"`,
				`src="https://www.youtube-nocookie.com/embed/onpage"`,
				`src="https://player.vimeo.com/video/76979871?dnt=1"`,
				`<video class="pr-video" controls preload="metadata" src="https://example.com/knead.mp4"></video>`,
				`<p>Before.</p>`,
				`<p>After.</p>`,
			} {
				if !strings.Contains(html, want) {
					t.Errorf("missing %q", want)
				}
			}
			if !strings.Contains(html, ".pr-video {") {
				t.Error("the video style is missing")
			}
			if strings.Contains(html, "raw HTML omitted") {
				t.Error("Hugo dropped the video as raw HTML")
			}
			if strings.Contains(html, "<!--pr-button") || strings.Contains(html, "<!--/pr-button") {
				t.Error("a marker is left in the page")
			}
		})
	}
}

func TestAlignedTextSurvivesInAColumn(t *testing.T) {
	const page = `---
title: Aligned
---

{{< pr-columns >}}
{{< pr-column >}}
{{< pr-align to="center" >}}
## Centered heading
{{< /pr-align >}}

Plain.

{{< pr-align to="right" >}}
> Right quote.
{{< /pr-align >}}
{{< /pr-column >}}

{{< pr-column >}}
Other.
{{< /pr-column >}}
{{< /pr-columns >}}
`
	for _, theme := range starterThemes {
		t.Run(theme, func(t *testing.T) {
			raw, err := os.ReadFile(buildPageSite(t, theme, page))
			if err != nil {
				t.Fatal(err)
			}
			html := string(raw)
			for _, want := range []string{
				`<div class="pr-align-center">`,
				`Centered heading</h2>`,
				`<p>Plain.</p>`,
				`<div class="pr-align-right">`,
				`Right quote.`,
				`.pr-align-center {`,
			} {
				if !strings.Contains(html, want) {
					t.Errorf("missing %q", want)
				}
			}
			if strings.Contains(html, "raw HTML omitted") {
				t.Error("Hugo dropped aligned text as raw HTML")
			}
			if strings.Contains(html, "<!--pr-button") || strings.Contains(html, "<!--/pr-button") {
				t.Error("a marker is left in the page")
			}
		})
	}
}

func buildThemedPages(t *testing.T, theme string, pages map[string]string, extraEnv ...string) string {
	t.Helper()
	hugo, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("hugo is not installed")
	}
	dir := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(dir, "Alt", "0.166.0"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "beach.jpg"), []byte("not really a jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range pages {
		path := filepath.Join(dir, "content", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(hugo, "--source", dir, "--destination", filepath.Join(dir, "public"), "--theme", theme, "--cacheDir", t.TempDir())
	cmd.Env = append(os.Environ(), extraEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hugo: %v\n%s", err, out)
	}
	return filepath.Join(dir, "public")
}

func TestStudioHomeKeepsPageContentBelowTheCover(t *testing.T) {
	buildHome := func(t *testing.T, frontMatter string) string {
		public := buildThemedPages(t, "pageroom-studio", map[string]string{"_index.md": "---\ntitle: Home\n" + frontMatter + "---\n\nHome body.\n"})
		raw, err := os.ReadFile(filepath.Join(public, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	t.Run("with a cover", func(t *testing.T) {
		html := buildHome(t, "image: beach.jpg\nimageAlt: A sandy beach\n")
		_, afterOpen, found := strings.Cut(html, `<section class="home-hero">`)
		if !found {
			t.Fatal("no cover section")
		}
		hero, rest, _ := strings.Cut(afterOpen, "</section>")
		if !strings.Contains(hero, `alt="A sandy beach"`) {
			t.Error("the cover picture is not in the cover section")
		}
		if strings.Contains(hero, "Home body.") || !strings.Contains(rest, "Home body.") {
			t.Error("the page content is not below the cover")
		}
	})

	t.Run("without a cover", func(t *testing.T) {
		html := buildHome(t, "")
		if strings.Contains(html, `class="home-hero"`) {
			t.Error("an empty cover section is on the page")
		}
		if !strings.Contains(html, "Home body.") {
			t.Error("the page content is missing")
		}
	})
}

func TestStarterThemesUseImageAltAsAltText(t *testing.T) {
	const alt = `alt="A sandy beach"`
	pages := map[string]string{
		"_index.md":        "---\ntitle: Home\nimage: beach.jpg\nimageAlt: A sandy beach\n---\n\nHome.\n",
		"landing/index.md": "---\ntitle: Landing\nimage: beach.jpg\nimageAlt: A sandy beach\n---\n\nBody.\n",
		"posts/_index.md":  "---\ntitle: Posts\n---\n",
		"posts/first.md":   "---\ntitle: First\ndate: 2026-01-01\nimage: beach.jpg\nimageAlt: A sandy beach\n---\n\nBody.\n",
	}
	wantFiles := map[string][]string{
		"pageroom-paper":  {"landing/index.html"},
		"pageroom-soft":   {"landing/index.html", "posts/index.html", "index.html"},
		"pageroom-studio": {"landing/index.html", "posts/index.html", "index.html"},
		"pageroom-tinted": {"landing/index.html", "posts/index.html", "index.html"},
	}
	for _, theme := range starterThemes {
		t.Run(theme, func(t *testing.T) {
			public := buildThemedPages(t, theme, pages)
			for _, name := range wantFiles[theme] {
				raw, err := os.ReadFile(filepath.Join(public, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(raw), alt) {
					t.Errorf("%s has no alt text:\n%s", name, raw)
				}
			}
		})
	}
}

func TestAddShortcodeKeepsExistingTemplate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(dir, "Blocks", "0.166.0"); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(dir, "layouts", "shortcodes", "pr-hero.html")
	if err := os.MkdirAll(filepath.Dir(custom), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(custom, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := site.Open(dir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddShortcode("pr-hero"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(custom); string(got) != "mine" {
		t.Fatalf("template overwritten: %q", got)
	}
}

func findBrowser() string {
	if root := os.Getenv("PLAYWRIGHT_BROWSERS_PATH"); root != "" {
		if matches, _ := filepath.Glob(filepath.Join(root, "chromium_headless_shell-*", "*", "chrome-headless-shell")); len(matches) > 0 {
			return matches[0]
		}
	}
	for _, name := range []string{"chrome-headless-shell", "chromium", "google-chrome", "chrome", "/Applications/Chromium.app/Contents/MacOS/Chromium", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

const phoneProbe = `<script>document.documentElement.dataset.probe = document.documentElement.scrollWidth + "/" + document.documentElement.clientWidth + "/" + window.innerWidth + "/" + Array.from(document.querySelectorAll(".pr-column")).every((column, index, all) => index === 0 || column.offsetTop > all[index - 1].offsetTop)</script>`

const desktopProbe = `<script>const wide = document.querySelector(".pr-columns-2-1").children; const three = document.querySelector(".pr-columns-1-1-1").children; document.documentElement.dataset.probe = (Math.round(wide[0].offsetWidth / wide[1].offsetWidth) === 2) + "/" + (wide[1].getBoundingClientRect().top > wide[0].getBoundingClientRect().top && wide[1].getBoundingClientRect().bottom === wide[0].getBoundingClientRect().bottom) + "/" + (three[0].offsetTop === three[2].offsetTop && three[0].offsetLeft < three[1].offsetLeft && three[1].offsetLeft < three[2].offsetLeft)</script>`

func requireBrowser(t *testing.T) string {
	t.Helper()
	browser := findBrowser()
	if browser == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("no Chromium or Chrome installed")
		}
		t.Skip("no Chromium or Chrome installed")
	}
	return browser
}

func probeBlocks(t *testing.T, theme, windowSize, probe string) []byte {
	t.Helper()
	requireBrowser(t)
	return probePage(t, buildBlocksSite(t, theme, "HUGO_RELATIVEURLS=true"), windowSize, probe)
}

func probePage(t *testing.T, page, windowSize, probe string) []byte {
	t.Helper()
	browser := requireBrowser(t)
	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, append(raw, probe...), 0o644); err != nil {
		t.Fatal(err)
	}

	// chrome-headless-shell honours --window-size=360,800 exactly, unlike the full
	// Chromium whose headless window stays at about 500px wide. --single-process
	// is what lets it start inside the Claude Code sandbox, and HUGO_RELATIVEURLS
	// lets the built page load its CSS from file://.
	args := []string{"--no-sandbox", "--single-process", "--disable-gpu", "--user-data-dir=" + t.TempDir(), "--window-size=" + windowSize, "--dump-dom", "file://" + page}
	if filepath.Base(browser) != "chrome-headless-shell" {
		args = append([]string{"--headless=new"}, args...)
	}
	out, err := exec.Command(browser, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, out)
	}
	return out
}

func TestHomeCardsDoNotScrollSideways(t *testing.T) {
	longUrl := "https://example.com/" + strings.Repeat("a", 180)
	pages := map[string]string{
		"_index.md":       "---\ntitle: Home\n---\n\nHome.\n",
		"posts/_index.md": "---\ntitle: Posts\n---\n",
		"posts/first.md":  "---\ntitle: First\ndate: 2026-01-01\nimage: beach.jpg\n---\n\n" + longUrl + "\n",
	}
	for _, theme := range starterThemes {
		for _, windowSize := range []string{"360,800", "1200,800"} {
			t.Run(theme+"/"+windowSize, func(t *testing.T) {
				requireBrowser(t)
				public := buildThemedPages(t, theme, pages, "HUGO_RELATIVEURLS=true")
				out := probePage(t, filepath.Join(public, "index.html"), windowSize, phoneProbe)
				match := regexp.MustCompile(`data-probe="(\d+)/(\d+)/(\d+)/`).FindSubmatch(out)
				if match == nil {
					t.Fatalf("no probe result in\n%s", out)
				}
				if string(match[1]) != string(match[2]) {
					t.Errorf("home page is %spx wide in a %spx viewport", match[1], match[2])
				}
			})
		}
	}
}

func TestColumnLayoutsOnADesktop(t *testing.T) {
	for _, theme := range starterThemes {
		t.Run(theme, func(t *testing.T) {
			out := probeBlocks(t, theme, "1200,800", desktopProbe)
			if !strings.Contains(string(out), `data-probe="true/true/true"`) {
				t.Errorf("want a 2:1 ratio, a bottom-aligned column and three columns in a row, got %s", regexp.MustCompile(`data-probe="[^"]*"`).Find(out))
			}
		})
	}
}

func TestBlocksDoNotScrollSidewaysOnAPhone(t *testing.T) {
	for _, theme := range starterThemes {
		t.Run(theme, func(t *testing.T) {
			out := probeBlocks(t, theme, "360,800", phoneProbe)
			match := regexp.MustCompile(`data-probe="(\d+)/(\d+)/(\d+)/(true|false)"`).FindSubmatch(out)
			if match == nil {
				t.Fatalf("no probe result in\n%s", out)
			}
			if string(match[3]) != "360" {
				t.Fatalf("viewport is %spx wide, want 360px", match[3])
			}
			if string(match[4]) != "true" {
				t.Error("the columns are side by side, want them stacked")
			}
			if string(match[1]) != string(match[2]) {
				t.Errorf("page is %spx wide in a %spx viewport", match[1], match[2])
			}
		})
	}
}

func openSiteWithTemplate(t *testing.T, name, content string) (*site.Site, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(dir, "Blocks", "0.166.0"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "layouts", "shortcodes", name+".html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := site.Open(dir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func shippedVersion(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUpdateShortcodesReplacesShippedVersion(t *testing.T) {
	older := shippedVersion(t, "pr-hero-e211718.html")
	s, path := openSiteWithTemplate(t, "pr-hero", older)
	updated, err := s.UpdateShortcodes()
	if err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile("shortcodes/pr-hero.html")
	if got, _ := os.ReadFile(path); string(got) != string(current) {
		t.Fatalf("older template not replaced")
	}
	if len(updated) != 1 || updated[0] != "pr-hero" {
		t.Fatalf("updated = %v", updated)
	}
}

func TestUpdateShortcodesKeepsEditedFile(t *testing.T) {
	edited := shippedVersion(t, "pr-hero-e211718.html") + "<!-- mine -->"
	s, path := openSiteWithTemplate(t, "pr-hero", edited)
	updated, err := s.UpdateShortcodes()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != edited || len(updated) != 0 {
		t.Fatalf("edited template touched: %q %v", got, updated)
	}
}

func TestUpdateShortcodesDoesNotCreateMissingFile(t *testing.T) {
	s, path := openSiteWithTemplate(t, "pr-hero", "")
	if _, err := s.UpdateShortcodes(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing template was created: %v", err)
	}
}

func TestEveryCurrentTemplateIsInTheShippedHashes(t *testing.T) {
	files, err := filepath.Glob("shortcodes/pr-*.html")
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates: %v", err)
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".html")
		data, _ := os.ReadFile(file)
		sum := sha256.Sum256(data)
		if !slices.Contains(site.ShippedShortcodeHashes[name], hex.EncodeToString(sum[:])) {
			t.Errorf("%s: current hash missing from shippedShortcodeHashes", name)
		}
	}
}
