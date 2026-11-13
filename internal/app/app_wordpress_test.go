package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
