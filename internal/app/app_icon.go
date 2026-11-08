package app

import (
	"encoding/base64"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/myrheimb/pageroom/internal/favicon"
	"github.com/myrheimb/pageroom/internal/fsutil"
	"github.com/myrheimb/pageroom/internal/media"
	"github.com/myrheimb/pageroom/internal/site"
)

type SiteIcon struct {
	Image  string `json:"image"`  // data URL
	Letter bool   `json:"letter"` // drawn from the title, not chosen
}

func (a *App) GetSiteIcon() (*SiteIcon, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	if icon := ownIcon(s); icon != nil {
		return iconState(icon), nil
	}
	icons, err := favicon.Letter(a.letterInputs(s))
	if err != nil {
		return nil, err
	}
	return iconState(icons.Favicon), nil
}

// ChooseSiteIcon makes the site's icon from an image the owner picks. It
// returns nil if they cancel.
func (a *App) ChooseSiteIcon() (*SiteIcon, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	path, err := application.Get().Dialog.OpenFile().
		SetTitle("Choose an icon").
		AddFilter("Images", "*.jpg;*.jpeg;*.png;*.gif;*.webp;*.JPG;*.JPEG;*.PNG").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > media.MaxUpload {
		return nil, &AppError{Code: "file-too-large", Message: "This file is too large (max 60 MB)."}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	icons, err := favicon.FromImage(data)
	if err != nil {
		return nil, &AppError{Code: "icon-format", Message: "Choose a JPEG, PNG, GIF or WebP image for the icon."}
	}
	return writeIcons(s, icons)
}

// UseLetterIcon goes back to the icon drawn from the site's title.
func (a *App) UseLetterIcon() (*SiteIcon, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	icons, err := favicon.Letter(a.letterInputs(s))
	if err != nil {
		return nil, err
	}
	return writeIcons(s, icons)
}

// updateLetterIcon redraws the letter icon before publishing, so it follows
// the title and primary color. Chosen icons are left alone.
func (a *App) updateLetterIcon(s *site.Site) error {
	if ownIcon(s) != nil {
		return nil
	}
	title, background := a.letterInputs(s)
	current, _ := os.ReadFile(filepath.Join(s.Root, favicon.File))
	if favicon.SameLetter(current, title, background) {
		return nil
	}
	icons, err := favicon.Letter(title, background)
	if err != nil {
		return err
	}
	_, err = writeIcons(s, icons)
	return err
}

// ownIcon is the site's or theme's favicon, unless it's a letter icon. The
// touch icon goes with it: Pageroom leaves both alone or writes both.
func ownIcon(s *site.Site) []byte {
	paths := []string{filepath.Join(s.Root, favicon.File)}
	if theme := s.Config.Theme(); theme != "" {
		paths = append(paths, filepath.Join(s.Root, "themes", theme, favicon.File))
	}
	for _, path := range paths {
		if icon, err := os.ReadFile(path); err == nil {
			if favicon.IsLetter(icon) {
				return nil
			}
			return icon
		}
	}
	return nil
}

func (a *App) letterInputs(s *site.Site) (title, background string) {
	a.mu.Lock()
	style := a.style
	a.mu.Unlock()
	if style != nil {
		background = style.Load().Tokens.Primary
	}
	if background == "" {
		background = favicon.DefaultBackground
	}
	return s.Config.String("title"), background
}

// writeIcons writes the favicon last, so if writing the touch icon fails,
// the next publish still sees an outdated letter and tries again.
func writeIcons(s *site.Site, icons *favicon.Icons) (*SiteIcon, error) {
	if err := os.MkdirAll(filepath.Join(s.Root, "static"), 0o755); err != nil {
		return nil, err
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(s.Root, favicon.TouchFile), icons.Touch, 0o644); err != nil {
		return nil, err
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(s.Root, favicon.File), icons.Favicon, 0o644); err != nil {
		return nil, err
	}
	return iconState(icons.Favicon), nil
}

func iconState(icon []byte) *SiteIcon {
	return &SiteIcon{Image: "data:image/x-icon;base64," + base64.StdEncoding.EncodeToString(icon), Letter: favicon.IsLetter(icon)}
}
