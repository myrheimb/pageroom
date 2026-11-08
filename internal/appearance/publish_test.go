package appearance

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	stylesheetHref = regexp.MustCompile(`href="([^"]+\.css)"`)
	cssUrl         = regexp.MustCompile(`url\(\s*["']?([^"')]+)["']?\s*\)`)
)

// Runs only where a Hugo binary is installed (CI installs one).
func TestPublishedFontUrlsResolveUnderSubfolder(t *testing.T) {
	bin, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("hugo not installed")
	}

	const subfolder = "/blog/"
	themes := map[string]string{
		"pageroom stylesheet":   `<html><head>{{ partial "extend_head.html" . }}</head></html>`,
		"theme with custom.css": `<html><head>{{ with resources.Get "css/custom.css" }}<link rel="stylesheet" href="{{ (. | fingerprint).RelPermalink }}">{{ end }}</head></html>`,
	}
	for name, layout := range themes {
		t.Run(name, func(t *testing.T) {
			root := site(t, map[string]string{
				"hugo.toml":                  "baseURL = 'https://example.com" + subfolder + "'\n",
				"layouts/index.html":         layout,
				"static/fonts/manrope.woff2": "font",
			})
			tokens := Tokens{BodyFont: "Manrope", HeadingFont: "Manrope", Fonts: []SiteFont{manrope}, Radius: -1}
			if _, err := (&Engine{Root: root}).Save(Style{Tokens: tokens}); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(bin, "--source", root, "--destination", filepath.Join(root, "public"), "--quiet").CombinedOutput(); err != nil {
				t.Fatalf("hugo: %v\n%s", err, out)
			}

			page, err := os.ReadFile(filepath.Join(root, "public", "index.html"))
			if err != nil {
				t.Fatal(err)
			}

			match := stylesheetHref.FindSubmatch(page)
			if match == nil || !strings.HasPrefix(string(match[1]), subfolder) {
				t.Fatalf("no stylesheet under %s in:\n%s", subfolder, page)
			}

			cssPath := strings.TrimPrefix(string(match[1]), subfolder)
			css, err := os.ReadFile(filepath.Join(root, "public", filepath.FromSlash(cssPath)))
			if err != nil {
				t.Fatal(err)
			}

			urls := cssUrl.FindAllStringSubmatch(string(css), -1)
			if len(urls) == 0 {
				t.Fatalf("no url(...) in:\n%s", css)
			}

			for _, found := range urls {
				if strings.HasPrefix(found[1], "/") || strings.Contains(found[1], "://") {
					t.Errorf("url %q does not stay under the subfolder", found[1])
					continue
				}

				target := path.Join(path.Dir(cssPath), found[1])
				if _, err := os.Stat(filepath.Join(root, "public", filepath.FromSlash(target))); err != nil {
					t.Errorf("url %q points to %s, which was not published", found[1], target)
				}
			}
		})
	}
}
