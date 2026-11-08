package app

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/myrheimb/pageroom/internal/favicon"
	"github.com/myrheimb/pageroom/internal/gitsync"
	"github.com/myrheimb/pageroom/internal/media"
	"github.com/myrheimb/pageroom/internal/secrets"
	"github.com/myrheimb/pageroom/internal/site"
)

func (a *App) gitOptions() gitsync.Options {
	return gitsync.Options{
		Auth:     a.authFor,
		Redactor: redactor,
		Author: func() (string, string) {
			d := a.settings.Get()
			return d.Name, d.Email
		},
	}
}

func isSSH(remote string) bool {
	return strings.HasPrefix(remote, "ssh://") || (strings.Contains(remote, "@") && !strings.Contains(remote, "://"))
}

// authFor supplies credentials for git operations. Secrets go straight from
// the keychain into the HTTP client and never reach the UI.
func (a *App) authFor(remote string) (transport.AuthMethod, error) {
	if isSSH(remote) {
		user := "git"
		if at := strings.Index(remote, "@"); at > 0 && !strings.Contains(remote[:at], "/") {
			user = strings.TrimPrefix(remote[:at], "ssh://")
		}
		auth, err := gitssh.NewSSHAgentAuth(user)
		if err != nil {
			return nil, &AppError{Code: "ssh-key", Message: "This site uses an SSH address, but no SSH key is available. Ask your developer, or switch the site to its HTTPS address."}
		}
		return auth, nil
	}
	c, err := a.creds.Get(remote)
	if errors.Is(err, secrets.ErrNotFound) {
		return nil, nil // try anonymously, since public repositories can be read
	}
	if err != nil {
		return nil, err
	}
	if c.Kind == "oauth" && !c.Expires.IsZero() && time.Until(c.Expires) < time.Minute && c.RefreshToken != "" {
		fresh, err := a.github.Refresh(a.ctx, *c)
		if err != nil {
			return nil, &gitsync.UserError{Code: "auth", Msg: err.Error()}
		}
		if err := a.creds.Save(remote, fresh); err != nil {
			return nil, err
		}
		c = &fresh
	}
	user := c.Username
	if user == "" {
		user = "x-access-token"
	}
	return &githttp.BasicAuth{Username: user, Password: c.Secret}, nil
}

func (a *App) requireRepo() (*gitsync.Repo, error) {
	if _, err := a.requireSite(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.repo == nil {
		return nil, &AppError{Code: "no-repo", Message: "This site isn't connected to an online repository yet."}
	}
	return a.repo, nil
}

// ChangeView is an unpublished change described for people.
type ChangeView struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"`  // added, modified, deleted
	Type  string `json:"type"`  // page, image, settings, appearance, theme, file
	Label string `json:"label"` // "About us", "beach.jpg"
}

func (a *App) GetChanges() ([]ChangeView, error) {
	repo, err := a.requireRepo()
	if err != nil {
		var ae *AppError
		if errors.As(err, &ae) && ae.Code == "no-repo" {
			return nil, nil
		}
		return nil, err
	}
	s, _ := a.requireSite()
	changes, err := repo.Changes()
	if err != nil {
		return nil, err
	}
	out := make([]ChangeView, 0, len(changes))
	for _, c := range changes {
		out = append(out, describe(s, c))
	}
	return out, nil
}

func describe(s *site.Site, c gitsync.Change) ChangeView {
	v := ChangeView{Path: c.Path, Kind: string(c.Kind), Type: "file", Label: c.Path}
	switch {
	case strings.HasPrefix(c.Path, "themes/"):
		name, _, _ := strings.Cut(strings.TrimPrefix(c.Path, "themes/"), "/")
		v.Type, v.Label = "theme", "Theme “"+name+"”"
	case media.IsImage(c.Path):
		v.Type, v.Label = "image", path.Base(c.Path)
	case strings.HasPrefix(c.Path, s.ContentDir+"/") && (strings.HasSuffix(c.Path, ".md") || strings.HasSuffix(c.Path, ".markdown")):
		v.Type = "page"
		v.Label = path.Base(strings.TrimSuffix(c.Path, path.Ext(c.Path)))
		if p, err := s.GetPage(c.Path); err == nil {
			if t, _ := p.Fields["title"].(string); t != "" {
				v.Label = t
			}
		}
	case c.Path == s.Config.Path || strings.HasPrefix(c.Path, "config/") || c.Path == favicon.File || c.Path == favicon.TouchFile:
		v.Type, v.Label = "settings", "Site settings & menus"
	case strings.HasSuffix(c.Path, "css/pageroom.css") || strings.HasSuffix(c.Path, "css/custom.css") || strings.HasPrefix(c.Path, "layouts/partials/"):
		v.Type, v.Label = "appearance", "Appearance"
	}
	return v
}

// SummarizeChanges writes a default publish message like
// "Edit “About us” and add 2 images".
func SummarizeChanges(cs []ChangeView) string {
	var pages []string
	images, other := 0, 0
	appearance, settings := false, false
	for _, c := range cs {
		switch c.Type {
		case "page":
			pages = append(pages, "“"+c.Label+"”")
		case "image":
			images++
		case "appearance", "theme":
			appearance = true
		case "settings":
			settings = true
		default:
			other++
		}
	}
	var parts []string
	switch {
	case len(pages) == 1:
		parts = append(parts, "Update "+pages[0])
	case len(pages) > 1 && len(pages) <= 3:
		parts = append(parts, "Update "+strings.Join(pages, ", "))
	case len(pages) > 3:
		parts = append(parts, fmt.Sprintf("Update %d pages", len(pages)))
	}
	if images > 0 {
		parts = append(parts, fmt.Sprintf("%d image%s", images, plural(images)))
	}
	if appearance {
		parts = append(parts, "appearance")
	}
	if settings {
		parts = append(parts, "site settings")
	}
	if other > 0 {
		parts = append(parts, fmt.Sprintf("%d other file%s", other, plural(other)))
	}
	if len(parts) == 0 {
		return "Update site"
	}
	out := strings.Join(parts, ", ")
	if len(pages) == 0 {
		out = "Update " + out
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (a *App) DefaultPublishMessage() (string, error) {
	cs, err := a.GetChanges()
	if err != nil {
		return "", err
	}
	return SummarizeChanges(cs), nil
}

// PendingPublish is the message of a publish that didn't finish uploading,
// so the publish dialog can offer to retry it as it was.
func (a *App) PendingPublish() (string, error) {
	repo, err := a.requireRepo()
	if err != nil {
		return "", err
	}
	return repo.PendingPublish()
}

func (a *App) Sync() (*gitsync.Outcome, error) {
	repo, err := a.requireRepo()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	out, err := repo.Sync(ctx)
	a.afterSync(out)
	return out, err
}

// Publish sends local changes live. The identity must be set first.
func (a *App) Publish(message string) (*gitsync.Outcome, error) {
	repo, err := a.requireRepo()
	if err != nil {
		return nil, err
	}
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	if err := a.updateLetterIcon(s); err != nil {
		return nil, err
	}
	if direct := directUpload(repo); direct != nil {
		if strings.TrimSpace(message) == "" {
			message, _ = a.DefaultPublishMessage()
		}
		return a.publishToCloudflare(s, repo, direct, message)
	}
	if d := a.settings.Get(); d.Name == "" {
		return nil, &AppError{Code: "need-identity", Message: "Please tell us your name so teammates can see who published."}
	}
	if repo.RemoteURL() == "" {
		return nil, &AppError{Code: "no-remote", Message: "Connect this site to GitHub (or another git host) before publishing."}
	}
	if strings.TrimSpace(message) == "" {
		message, _ = a.DefaultPublishMessage()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	out, err := repo.Publish(ctx, message)
	a.afterSync(out)
	return out, err
}

// Resolve continues a Sync/Publish after the user chose what to keep.
func (a *App) Resolve(choices map[string]gitsync.Choice) (*gitsync.Outcome, error) {
	repo, err := a.requireRepo()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	out, err := repo.Resolve(ctx, choices)
	a.afterSync(out)
	return out, err
}

func (a *App) afterSync(out *gitsync.Outcome) {
	if out == nil || len(out.Incoming) == 0 {
		return
	}
	if s, err := a.requireSite(); err == nil {
		// The sync itself succeeded. A config that no longer parses surfaces on the next site load.
		_ = s.Reload()
	}
}

// Discard restores the published version of a file.
func (a *App) Discard(path string) error {
	repo, err := a.requireRepo()
	if err != nil {
		return err
	}
	return repo.Discard(path)
}

func (a *App) History(path string) ([]gitsync.Revision, error) {
	repo, err := a.requireRepo()
	if err != nil {
		return nil, err
	}
	return repo.History(path, 50)
}

func (a *App) RevisionContent(path, commit string) (string, error) {
	repo, err := a.requireRepo()
	if err != nil {
		return "", err
	}
	b, err := repo.FileAt(commit, path)
	return string(b), err
}

// RestoreRevision puts an older version back as an unpublished change.
func (a *App) RestoreRevision(path, commit string) (*site.Page, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	repo, err := a.requireRepo()
	if err != nil {
		return nil, err
	}
	b, err := repo.FileAt(commit, path)
	if err != nil {
		return nil, err
	}
	cur, err := s.GetPage(path)
	if err != nil {
		return nil, err
	}
	return s.WriteRaw(path, cur.Hash, b)
}

// ConnectRemote links a local-only site (e.g. a new one) to an online
// repository, so the first Publish uploads it.
func (a *App) ConnectRemote(remoteURL string) (*GitState, error) {
	repo, err := a.requireRepo()
	if err != nil {
		return nil, err
	}
	if directUpload(repo) != nil {
		return nil, &AppError{Code: "cloudflare-direct", Message: "Stop hosting on Cloudflare before connecting an online repository."}
	}
	clean, user, pass, changed := secrets.StripURLCredentials(strings.TrimSpace(remoteURL))
	if changed {
		if err := a.creds.Save(clean, secrets.Credential{Kind: "token", Username: user, Secret: pass}); err != nil {
			return nil, err
		}
	}
	if err := repo.SetRemote(clean); err != nil {
		return nil, err
	}
	g := a.gitState()
	return &g, nil
}

// DisconnectRemote stops publishing to the online repository, along with
// the hosting setup that belongs to it. Nothing online is deleted.
func (a *App) DisconnectRemote() error {
	repo, err := a.requireRepo()
	if err != nil {
		return err
	}
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	if pages := githubPages(repo.RemoteURL(), s.Root); pages != nil && pages.SetUp {
		if err := a.RemoveHosting("github-pages"); err != nil {
			return err
		}
	}
	if worker := cloudflare(repo.RemoteURL(), s.Root); worker != nil && worker.SetUp {
		if err := a.RemoveHosting("cloudflare"); err != nil {
			return err
		}
	}
	return repo.RemoveRemote()
}
