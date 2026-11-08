package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/myrheimb/pageroom/internal/hugo"
	"github.com/myrheimb/pageroom/internal/site"
)

func (a *App) workDir(s *site.Site) string {
	h := sha256.Sum256([]byte(s.Root))
	return filepath.Join(a.dirs.Cache, "preview", hex.EncodeToString(h[:6]))
}

// hugoBin picks the Hugo to run for a site, never downloading by itself. If
// Pageroom's own copy is needed but missing, it returns a "hugo-download"
// error so the UI can ask first. note is set when that copy replaces the
// user's installed Hugo.
func (a *App) hugoBin(s *site.Site) (bin, note string, err error) {
	c := a.hugo.Choose(hugo.SiteVersion(s.Root, s.Prefs.Hugo.Version), hugo.FindInstalled())
	if c.Path == "" {
		details := map[string]string{"version": c.Version}
		if c.Installed != nil {
			details["installedVersion"] = c.Installed.Version
			if !c.Installed.Extended {
				details["installedWithoutExtended"] = "true"
			}
		}
		return "", "", &AppError{Code: "hugo-download", Message: fmt.Sprintf("Pageroom needs Hugo %s to show previews of this site.", c.Version), Details: details}
	}
	if c.Source == "pageroom" && c.Installed != nil {
		mine := c.Installed.Version
		if c.Installed.Version == c.Version && !c.Installed.Extended {
			mine += " without the “extended” features"
		}
		note = fmt.Sprintf("Using Pageroom's own copy of Hugo %s, because this site needs that version and the Hugo installed on your computer is %s.", c.Version, mine)
	}
	return c.Path, note, nil
}

// DownloadHugo fetches the Hugo version the site needs into Pageroom's
// private cache. The UI calls it after the user agreed, and progress arrives
// as "hugo:download" events.
func (a *App) DownloadHugo() error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Minute)
	defer cancel()
	if _, err := a.hugo.Download(ctx, hugo.SiteVersion(s.Root, s.Prefs.Hugo.Version)); err != nil {
		return &AppError{Code: "hugo", Message: redactor.String(err.Error())}
	}
	return nil
}

type PreviewState struct {
	URL     string   `json:"url"`
	Running bool     `json:"running"`
	Errors  []string `json:"errors,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// StartPreview starts (or returns) the live preview server. If Hugo must be
// downloaded first it returns a "hugo-download" error (see hugoBin).
func (a *App) StartPreview() (*PreviewState, error) {
	// One start at a time: two screens asking at once must share a server,
	// not start two and leak one.
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	if a.preview != nil && a.preview.Running() {
		p, note := a.preview, a.previewNote
		a.mu.Unlock()
		return &PreviewState{URL: p.URL, Running: true, Errors: p.Errors(), Note: note}, nil
	}
	a.mu.Unlock()
	bin, note, err := a.hugoBin(s)
	if err != nil {
		return nil, err
	}
	// Hugo only watches folders that exist at start, and Appearance or image
	// uploads may create these later. Git ignores empty folders.
	for _, d := range []string{"layouts", "assets", "static"} {
		_ = os.MkdirAll(filepath.Join(s.Root, d), 0o755)
	}
	srv, err := hugo.StartServer(a.ctx, bin, s.Root, a.workDir(s))
	if err != nil {
		return nil, &AppError{Code: "preview", Message: redactor.String(err.Error())}
	}
	a.mu.Lock()
	if a.site != s { // site closed while starting
		a.mu.Unlock()
		srv.Stop()
		return nil, &AppError{Code: "no-site", Message: "No site is open."}
	}
	a.preview, a.previewNote = srv, note
	a.mu.Unlock()
	return &PreviewState{URL: srv.URL, Running: true, Note: note}, nil
}

// PreviewStatus reports problems Hugo found while rebuilding.
func (a *App) PreviewStatus() PreviewState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.preview == nil {
		return PreviewState{}
	}
	return PreviewState{URL: a.preview.URL, Running: a.preview.Running(), Errors: a.preview.Errors()}
}

func (a *App) StopPreview() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.preview.Stop()
	a.preview = nil
}
