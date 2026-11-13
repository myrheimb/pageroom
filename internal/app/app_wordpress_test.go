package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrheimb/pageroom/internal/settings"
	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/starter"
)

const coverExport = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:wp="http://wordpress.org/export/1.2/">
<channel>
<title>Bakery</title>
<link>https://bakery.example/</link>
<description>Bread</description>
</channel>
</rss>`

type homePageFetcher struct{ page string }

func (f homePageFetcher) Get(_ context.Context, rawURL string) ([]byte, error) {
	if rawURL == "https://bakery.example/" {
		return []byte(f.page), nil
	}
	return nil, errors.New("404")
}

func newWordPressApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := settings.Open(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *settings.Data) { data.SitesFolder = filepath.Join(dir, "sites") }); err != nil {
		t.Fatal(err)
	}
	app := &App{
		ctx:            context.Background(),
		dirs:           settings.Dirs{Data: filepath.Join(dir, "data")},
		settings:       store,
		wordPressFetch: homePageFetcher{`<html><body><div class="wp-block-cover"></div></body></html>`},
	}
	t.Cleanup(app.closeSite)
	exportPath := filepath.Join(dir, "export.xml")
	if err := os.WriteFile(exportPath, []byte(coverExport), 0o644); err != nil {
		t.Fatal(err)
	}
	return app, exportPath
}

func TestCreateSiteFromWordPressSavesThePickedTheme(t *testing.T) {
	app, exportPath := newWordPressApp(t)

	result, err := app.CreateSiteFromWordPress(exportPath, true)
	if err != nil {
		t.Fatal(err)
	}

	if result.Report.Theme != "pageroom-studio" {
		t.Fatalf("picked %q, want pageroom-studio", result.Report.Theme)
	}
	if result.Site.Info.Theme != "pageroom-studio" {
		t.Fatalf("new site has theme %q, want pageroom-studio", result.Site.Info.Theme)
	}
}

func TestImportWordPressKeepsTheSitesTheme(t *testing.T) {
	app, exportPath := newWordPressApp(t)
	root := filepath.Join(t.TempDir(), "site")
	if err := starter.CreateEmpty(root, "Bakery", "0.168.0"); err != nil {
		t.Fatal(err)
	}
	existing, err := site.Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.site = existing
	before := existing.Config.Theme()

	rep, err := app.ImportWordPress(exportPath, true)
	if err != nil {
		t.Fatal(err)
	}

	if rep.Theme == "" || rep.Theme == before {
		t.Fatalf("picked %q, want a theme other than the site's %q so the test can tell", rep.Theme, before)
	}
	if got := existing.Config.Theme(); got != before {
		t.Fatalf("site theme is %q after import, want %q", got, before)
	}
}

func TestCreateSiteFromWordPressWritesTheHomePage(t *testing.T) {
	staticFront := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:wp="http://wordpress.org/export/1.2/" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
<title>Bakery</title>
<link>https://bakery.example/</link>
<description>Bread</description>
<item>
<title>Welcome</title>
<content:encoded><![CDATA[Come in.]]></content:encoded>
<wp:post_id>5</wp:post_id>
<wp:post_name>welcome</wp:post_name>
<wp:status>publish</wp:status>
<wp:post_type>page</wp:post_type>
</item>
</channel>
</rss>`
	tests := []struct {
		name   string
		export string
		page   string
		want   string
	}{
		{"static front page", staticFront, `<body class="page-id-5">`, "Come in."},
		{"posts front page", coverExport, `<body class="home blog">`, "Bread"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, exportPath := newWordPressApp(t)
			app.wordPressFetch = homePageFetcher{test.page}
			if err := os.WriteFile(exportPath, []byte(test.export), 0o644); err != nil {
				t.Fatal(err)
			}

			result, err := app.CreateSiteFromWordPress(exportPath, true)
			if err != nil {
				t.Fatal(err)
			}

			home, err := os.ReadFile(filepath.Join(result.Site.Info.Root, "content", "_index.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(home), test.want) || strings.Contains(string(home), "Welcome to your new website") {
				t.Errorf("home page:\n%s", home)
			}
		})
	}
}
