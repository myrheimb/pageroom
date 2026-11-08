package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/myrheimb/pageroom/internal/appearance"
	"github.com/myrheimb/pageroom/internal/fsutil"
	"github.com/myrheimb/pageroom/internal/hugo"
	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/themes"
)

func (a *App) GetPage(path string) (*site.Page, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	return s.GetPage(path)
}

// SavePage saves edits. If the file changed on disk since it was opened,
// it returns a "changed-on-disk" error and the UI asks what to keep.
func (a *App) SavePage(req site.SaveRequest) (*site.Page, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	p, err := s.SavePage(req)
	var ce *site.ChangedOnDiskError
	if errors.As(err, &ce) {
		return nil, &AppError{Code: "changed-on-disk", Message: ce.Error(), Path: ce.Path}
	}
	return p, err
}

func (a *App) CreatePage(np site.NewPage) (string, error) {
	s, err := a.requireSite()
	if err != nil {
		return "", err
	}
	return s.CreatePage(np)
}

func (a *App) CreateSection(parent, title string) (string, error) {
	s, err := a.requireSite()
	if err != nil {
		return "", err
	}
	return s.CreateSection(parent, title)
}

func (a *App) MovePage(path, toSection string) (string, error) {
	s, err := a.requireSite()
	if err != nil {
		return "", err
	}
	return s.MovePage(path, toSection)
}

func (a *App) RenameSlug(path, slug string) (string, error) {
	s, err := a.requireSite()
	if err != nil {
		return "", err
	}
	return s.RenameSlug(path, slug)
}

// DeletePage moves the page to Pageroom's trash.
func (a *App) DeletePage(path string) error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	_, err = s.DeletePage(path)
	return err
}

func (a *App) ReorderPages(paths []string) error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	return s.Reorder(paths)
}

func (a *App) GetMenus() ([]site.Menu, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	return s.Menus()
}

func (a *App) SaveMenu(m site.Menu) error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	if err := s.SaveMenu(m); err != nil {
		return err
	}
	return s.Reload()
}

func (a *App) SaveSettings(n site.Settings) error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	n.FormAddress = strings.TrimSpace(n.FormAddress)
	n.ContactEmail = strings.TrimSpace(n.ContactEmail)
	if n.FormAddress != "" {
		if u, err := url.Parse(n.FormAddress); err != nil || u.Scheme != "https" || u.Host == "" {
			return &AppError{Code: "form-address", Message: "The form address must be a web address starting with https://."}
		}
	}
	if n.ContactEmail != "" {
		if address, err := mail.ParseAddress(n.ContactEmail); err != nil || address.Address != n.ContactEmail {
			return &AppError{Code: "contact-email", Message: "That doesn't look like an email address."}
		}
	}
	if err := s.SaveSettings(n); err != nil {
		return err
	}
	return s.Reload()
}

// AddContactForm makes sure the site can show the contact form the editor
// is about to insert.
func (a *App) AddContactForm() error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	return s.AddContactForm()
}

// AddPageBlock makes sure the site has the templates for a page block the
// editor is about to insert or align. The kinds are align, hero, cards and button.
func (a *App) AddPageBlock(kind string) error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	names, ok := pageBlockShortcodes[kind]
	if !ok {
		return &AppError{Code: "page-block", Message: "Unknown page block: " + kind}
	}
	for _, name := range names {
		if err := s.AddShortcode(name); err != nil {
			return err
		}
	}
	return nil
}

var pageBlockShortcodes = map[string][]string{
	"align":  {"pr-align"},
	"button": {"pr-button"},
	"cards":  {"pr-cards", "pr-card"},
	"hero":   {"pr-hero"},
}

type ThemeInfo struct {
	Name       string `json:"name"`
	Active     bool   `json:"active"`
	Screenshot string `json:"screenshot,omitempty"` // site-file path
	Desc       string `json:"description,omitempty"`
}

func (a *App) ListThemes() ([]ThemeInfo, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	active := s.Config.Theme()
	var out []ThemeInfo
	for _, t := range s.Themes() {
		ti := ThemeInfo{Name: t, Active: t == active}
		for _, shot := range []string{"images/screenshot.png", "images/tn.png", "screenshot.png"} {
			if _, err := os.Stat(filepath.Join(s.Root, "themes", t, shot)); err == nil {
				ti.Screenshot = "themes/" + t + "/" + shot
				break
			}
		}
		if b, err := os.ReadFile(filepath.Join(s.Root, "themes", t, "theme.toml")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "description" {
					ti.Desc = strings.Trim(strings.TrimSpace(v), `"'`)
				}
			}
		}
		out = append(out, ti)
	}
	return out, nil
}

// CheckTheme test-builds the site with another theme so the user sees
// problems before switching. Returns "" when the build succeeds.
func (a *App) CheckTheme(name string) (string, error) {
	s, err := a.requireSite()
	if err != nil {
		return "", err
	}
	bin, _, err := a.hugoBin(s)
	if err != nil {
		return "", err
	}
	if err := hugo.Check(a.ctx, bin, s.Root, a.workDir(s), name, ""); err != nil {
		return redactor.String(err.Error()), nil
	}
	return "", nil
}

// SwitchTheme changes the active theme (the previous one stays installed,
// so switching back is one click).
func (a *App) SwitchTheme(name string) error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	found := false
	for _, t := range s.Themes() {
		found = found || t == name
	}
	if !found {
		return &AppError{Code: "theme-not-installed", Message: "That theme isn't installed."}
	}
	st := s.Settings()
	st.Theme = name
	if err := s.SaveSettings(st); err != nil {
		return err
	}
	if err := s.Reload(); err != nil {
		return err
	}
	a.mu.Lock()
	a.style = &appearance.Engine{Root: s.Root, Theme: name, HeadHook: s.Prefs.Appearance.HeadHook}
	a.mu.Unlock()
	// Hugo only watches folders that existed when it started, so a new theme
	// needs a fresh preview.
	a.StopPreview()
	return nil
}

func (a *App) GalleryThemes() ([]themes.Theme, error) {
	index, err := themes.Catalog()
	return index.Themes, err
}

// themeTrial is how to undo trying a gallery theme.
type themeTrial struct {
	dir    string // the downloaded theme
	config string // the site's config file
	before []byte // its contents before the switch
}

// TryTheme downloads a gallery theme, test-builds the site with it and
// switches to it. If the build fails it returns the problems and leaves
// nothing behind. EndThemeTrial keeps the theme or undoes it all.
func (a *App) TryTheme(name string) (string, error) {
	s, err := a.requireSite()
	if err != nil {
		return "", err
	}
	index, err := themes.Catalog()
	if err != nil {
		return "", err
	}
	at := slices.IndexFunc(index.Themes, func(theme themes.Theme) bool { return theme.Name == name })
	if at < 0 {
		return "", &AppError{Code: "theme-not-in-gallery", Message: "That theme isn't in the gallery."}
	}
	dir := filepath.Join(s.Root, "themes", name)
	if fsutil.Exists(dir) {
		return "", &AppError{Code: "theme-exists", Message: fmt.Sprintf("Your site already has a theme called “%s”. You can switch to it in the list of installed themes.", name), Details: map[string]string{"name": name}}
	}
	// Ask about downloading Hugo before downloading the theme.
	if _, _, err := a.hugoBin(s); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	if err := themes.Install(ctx, http.DefaultClient, index.Themes[at], dir); err != nil {
		reason := redactor.String(err.Error())
		return "", &AppError{Code: "theme-download", Message: "Couldn't download the theme. Check your internet connection and try again. (" + reason + ")", Details: map[string]string{"reason": reason}}
	}
	config := filepath.Join(s.Root, s.Config.Path)
	before, err := os.ReadFile(config)
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	problems, err := a.CheckTheme(name)
	if err == nil && problems == "" {
		err = a.SwitchTheme(name)
	}
	if err != nil || problems != "" {
		os.RemoveAll(dir)
		return problems, err
	}

	a.mu.Lock()
	a.trial = &themeTrial{dir: dir, config: config, before: before}
	a.mu.Unlock()
	return "", nil
}

// EndThemeTrial keeps the theme being tried, or puts the site back exactly
// as it was and deletes the download.
func (a *App) EndThemeTrial(keep bool) error {
	a.mu.Lock()
	trial := a.trial
	a.trial = nil
	a.mu.Unlock()
	if trial == nil || keep {
		return nil
	}

	s, err := a.requireSite()
	if err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(trial.config, trial.before, 0o644); err != nil {
		return err
	}
	if err := s.Reload(); err != nil {
		return err
	}
	a.mu.Lock()
	a.style = &appearance.Engine{Root: s.Root, Theme: s.Config.Theme(), HeadHook: s.Prefs.Appearance.HeadHook}
	a.mu.Unlock()
	a.StopPreview()
	return os.RemoveAll(trial.dir)
}

type StyleState struct {
	Style appearance.Style  `json:"style"`
	Hook  appearance.Hook   `json:"hook"`
	Fonts map[string]string `json:"fonts"`
}

func (a *App) GetStyle() (*StyleState, error) {
	if _, err := a.requireSite(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	e := a.style
	a.mu.Unlock()
	return &StyleState{Style: e.Load(), Hook: e.DetectHook(), Fonts: appearance.FontStacks}, nil
}

func (a *App) SaveStyle(st appearance.Style) (*appearance.Hook, error) {
	if _, err := a.requireSite(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	e := a.style
	a.mu.Unlock()
	h, err := e.Save(st)
	if err != nil {
		return nil, &AppError{Code: "invalid", Message: err.Error()}
	}
	return &h, nil
}
