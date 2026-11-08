package site_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

After.
`

var starterThemes = []string{"pageroom-paper", "pageroom-soft", "pageroom-studio", "pageroom-tinted"}

func buildBlocksSite(t *testing.T, theme string, extraEnv ...string) string {
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
	for _, name := range []string{"pr-align", "pr-button", "pr-card", "pr-cards", "pr-hero"} {
		if err := s.AddShortcode(name); err != nil {
			t.Fatal(err)
		}
	}
	pageDir := filepath.Join(dir, "content", "landing")
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pageDir, "index.md"), []byte(blocksPage), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(hugo, "--source", dir, "--destination", filepath.Join(dir, "public"), "--theme", theme, "--cacheDir", t.TempDir())
	cmd.Env = append(os.Environ(), extraEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hugo: %v\n%s", err, out)
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
			if got := strings.Count(html, ".pr-cards {"); got != 1 {
				t.Errorf("cards CSS written %d times, want 1", got)
			}
			if strings.Index(html, "Before.") > strings.Index(html, `<section class="pr-hero">`) || strings.Index(html, "After.") < strings.Index(html, `<a class="pr-button" href="/contact/">`) {
				t.Error("blocks are not between the paragraphs")
			}
		})
	}
}

func TestStarterThemesUseImageAltAsAltText(t *testing.T) {
	hugo, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("hugo is not installed")
	}
	const alt = `alt="A sandy beach"`
	pages := map[string]string{
		"_index.md":        "---\ntitle: Home\nimage: beach.jpg\nimageAlt: A sandy beach\n---\n\nHome.\n",
		"landing/index.md": "---\ntitle: Landing\nimage: beach.jpg\nimageAlt: A sandy beach\n---\n\nBody.\n",
		"posts/_index.md":  "---\ntitle: Posts\n---\n",
		"posts/first.md":   "---\ntitle: First\ndate: 2026-01-01\nimage: beach.jpg\nimageAlt: A sandy beach\n---\n\nBody.\n",
	}
	wantFiles := map[string][]string{
		"pageroom-paper":  {"landing/index.html"},
		"pageroom-soft":   {"landing/index.html", "posts/index.html"},
		"pageroom-studio": {"landing/index.html", "posts/index.html", "index.html"},
		"pageroom-tinted": {"landing/index.html", "posts/index.html"},
	}
	for _, theme := range starterThemes {
		t.Run(theme, func(t *testing.T) {
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
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("hugo: %v\n%s", err, out)
			}
			for _, name := range wantFiles[theme] {
				raw, err := os.ReadFile(filepath.Join(dir, "public", filepath.FromSlash(name)))
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

// chrome-headless-shell honours --window-size=360,800 exactly, unlike the full
// Chromium whose headless window stays at about 500px wide. --single-process
// is what lets it start inside the Claude Code sandbox, and HUGO_RELATIVEURLS lets
// the built page load its CSS from file://.
const phoneProbe = `<script>document.documentElement.dataset.probe = document.documentElement.scrollWidth + "/" + document.documentElement.clientWidth + "/" + window.innerWidth</script>`

func TestBlocksDoNotScrollSidewaysOnAPhone(t *testing.T) {
	browser := findBrowser()
	if browser == "" {
		t.Skip("no Chromium or Chrome installed")
	}
	for _, theme := range starterThemes {
		t.Run(theme, func(t *testing.T) {
			page := buildBlocksSite(t, theme, "HUGO_RELATIVEURLS=true")
			raw, err := os.ReadFile(page)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(page, append(raw, phoneProbe...), 0o644); err != nil {
				t.Fatal(err)
			}

			args := []string{"--no-sandbox", "--single-process", "--disable-gpu", "--user-data-dir=" + t.TempDir(), "--window-size=360,800", "--dump-dom", "file://" + page}
			if filepath.Base(browser) != "chrome-headless-shell" {
				args = append([]string{"--headless=new"}, args...)
			}
			out, err := exec.Command(browser, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("browser: %v\n%s", err, out)
			}
			match := regexp.MustCompile(`data-probe="(\d+)/(\d+)/(\d+)"`).FindSubmatch(out)
			if match == nil {
				t.Fatalf("no probe result in\n%s", out)
			}
			if string(match[3]) != "360" {
				t.Fatalf("viewport is %spx wide, want 360px", match[3])
			}
			if string(match[1]) != string(match[2]) {
				t.Errorf("page is %spx wide in a %spx viewport", match[1], match[2])
			}
		})
	}
}
