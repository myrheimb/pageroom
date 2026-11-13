package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/myrheimb/pageroom/internal/appearance"
	"github.com/myrheimb/pageroom/internal/fsutil"
	"github.com/myrheimb/pageroom/internal/gitsync"
	"github.com/myrheimb/pageroom/internal/hugo"
	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/starter"
	"github.com/myrheimb/pageroom/internal/wordpress"
)

func (a *App) PickWordPressFile() (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle("Choose your WordPress export file").
		AddFilter("WordPress export (*.xml)", "*.xml").
		PromptForSingleSelection()
}

func readExport(path string) (*wordpress.Export, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	e, err := wordpress.Parse(f)
	if err != nil {
		return nil, &AppError{Code: "invalid", Message: err.Error()}
	}
	return e, nil
}

// PeekWordPress summarizes an export before importing.
func (a *App) PeekWordPress(path string) (*wordpress.Summary, error) {
	e, err := readExport(path)
	if err != nil {
		return nil, err
	}
	s := e.Summarize()
	return &s, nil
}

func (a *App) runImport(s *site.Site, e *wordpress.Export, download, pickTheme bool) (*wordpress.Report, error) {
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	section := "posts"
	for _, c := range []string{"posts", "post", "blog", "news"} {
		if st, err := os.Stat(filepath.Join(s.Root, s.ContentDir, c)); err == nil && st.IsDir() {
			section = c
			break
		}
	}
	fetch := a.wordPressFetch
	if fetch == nil {
		fetch = wordpress.NewHTTPFetcher()
	}
	rep, err := wordpress.Import(ctx, e, wordpress.Options{
		Site: s, PostsSection: section, DownloadImages: download, Fetch: fetch,
		Progress: func(done, total int, label string) {
			if application.Get() == nil {
				return
			}
			application.Get().Event.Emit("wp:progress", map[string]any{"done": done, "total": total, "label": label})
		},
	})
	if err != nil {
		return rep, err
	}

	if pickTheme && rep.Theme != "" {
		st := s.Settings()
		st.Theme = rep.Theme
		if err := s.SaveSettings(st); err != nil {
			return rep, err
		}
		if err := s.Reload(); err != nil {
			return rep, err
		}
	}

	style := &appearance.Engine{Root: s.Root, Theme: s.Config.Theme(), HeadHook: s.Prefs.Appearance.HeadHook}
	untouched := appearance.Style{Tokens: appearance.Tokens{Radius: -1}}
	if rep.Style != nil && reflect.DeepEqual(style.Load(), untouched) {
		if _, err := style.Save(appearance.Style{Tokens: *rep.Style}); err != nil {
			return rep, err
		}
	}

	if rep.CustomCSS != "" {
		current := style.Load()
		if !strings.Contains(current.CustomCSS, rep.CustomCSS) {
			current.CustomCSS = strings.TrimSpace(current.CustomCSS + "\n\n" + rep.CustomCSS)
			if _, err := style.Save(current); err != nil {
				return rep, err
			}
		}
	}

	if rep.Icon != nil && ownIcon(s) == nil {
		if _, err := writeIcons(s, rep.Icon); err != nil {
			return rep, err
		}
	}

	if len(rep.ContactForms) > 0 {
		st := s.Settings()
		if st.FormAddress == "" && st.ContactEmail == "" && rep.ContactEmail != "" {
			st.ContactEmail = rep.ContactEmail
			if err := s.SaveSettings(st); err != nil {
				return rep, err
			}
		}
		if st.FormAddress == "" {
			rep.Warnings = append(rep.Warnings, contactFormWarning(rep.ContactForms, st.ContactEmail))
		}
	}
	return rep, nil
}

func contactFormWarning(titles []string, email string) string {
	verb := "has"
	if len(titles) > 1 {
		verb = "have"
	}
	w := fmt.Sprintf("“%s” now %s a contact form. Add a form address in Settings → Site so visitors can send messages from it.", strings.Join(titles, "”, “"), verb)
	if email == "" {
		return w + " Until then, it only shows in the preview."
	}
	return w + " Until then, it shows " + email + " instead."
}

// ImportWordPress imports an export into the open site. The imported pages
// show up as unpublished changes to review before publishing.
func (a *App) ImportWordPress(path string, downloadImages bool) (*wordpress.Report, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	e, err := readExport(path)
	if err != nil {
		return nil, err
	}
	return a.runImport(s, e, downloadImages, false)
}

type WordPressSiteResult struct {
	Site   *SiteState        `json:"site"`
	Report *wordpress.Report `json:"report"`
}

// CreateSiteFromWordPress makes a new site with the starter theme and the
// export's content, in one step.
func (a *App) CreateSiteFromWordPress(path string, downloadImages bool) (*WordPressSiteResult, error) {
	e, err := readExport(path)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(e.Title)
	if title == "" {
		title = "My website"
	}
	parent := a.settings.Get().SitesFolder
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	dest := fsutil.UniquePath(filepath.Join(parent, fsutil.Slugify(title)))
	if err := starter.CreateEmpty(dest, title, hugo.DefaultVersion); err != nil {
		return nil, err
	}
	s, err := site.Open(dest, filepath.Join(a.dirs.Data, "trash", fsutil.Slugify(title)))
	if err != nil {
		return nil, err
	}
	st := s.Settings()
	st.Description = e.Description
	if strings.HasPrefix(e.Link, "http") {
		st.BaseURL = strings.TrimRight(e.Link, "/") + "/"
	}
	if err := s.SaveSettings(st); err != nil {
		return nil, err
	}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	rep, err := a.runImport(s, e, downloadImages, true)
	if err != nil {
		return nil, err
	}
	repo, err := gitsync.Init(dest, a.gitOptions())
	if err != nil {
		return nil, err
	}
	if err := repo.CommitAll("Import “" + title + "” from WordPress"); err != nil {
		return nil, err
	}
	state, err := a.OpenSite(dest)
	if err != nil {
		return nil, err
	}
	return &WordPressSiteResult{Site: state, Report: rep}, nil
}
