package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/myrheimb/pageroom/internal/appearance"
	"github.com/myrheimb/pageroom/internal/fsutil"
	"github.com/myrheimb/pageroom/internal/gitsync"
	"github.com/myrheimb/pageroom/internal/hugo"
	"github.com/myrheimb/pageroom/internal/media"
	"github.com/myrheimb/pageroom/internal/secrets"
	"github.com/myrheimb/pageroom/internal/settings"
	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/starter"
	"github.com/myrheimb/pageroom/internal/update"
)

// redactor is shared by everything that might put text in front of the user.
var redactor = &secrets.Redactor{}

// App is bound to the frontend: its exported methods are the UI's API.
type App struct {
	ctx      context.Context
	dirs     settings.Dirs
	settings *settings.Store
	creds    *secrets.Store
	github   *secrets.GitHubAuth
	hugo     *hugo.Manager

	cloudflareHTTP *http.Client // nil uses the default. Tests replace it.

	mu          sync.Mutex
	previewMu   sync.Mutex
	site        *site.Site
	repo        *gitsync.Repo // nil when the folder isn't a git repository
	media       *media.Library
	style       *appearance.Engine
	preview     *hugo.Server
	previewNote string
	trial       *themeTrial
	watcher     *watcher
	signIn      context.CancelFunc

	updateOnce   sync.Once
	updateResult *update.Release

	pendingDevice *secrets.DeviceCode
	closing       bool
}

func NewApp() (*App, error) {
	dirs, err := settings.DefaultDirs()
	if err != nil {
		return nil, err
	}
	st, err := settings.Open(dirs.Config)
	if err != nil {
		return nil, err
	}
	return &App{
		dirs:     dirs,
		settings: st,
		creds:    &secrets.Store{Backend: secrets.Keyring{}, Redactor: redactor},
		github:   &secrets.GitHubAuth{ClientID: secrets.GitHubClientID},
		hugo:     &hugo.Manager{CacheDir: filepath.Join(dirs.Cache, "hugo")},
	}, nil
}

func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.ctx = ctx
	a.hugo.Progress = func(done, total int64) {
		application.Get().Event.Emit("hugo:download", map[string]int64{"done": done, "total": total})
	}
	return nil
}

func (a *App) ServiceShutdown() error {
	a.closeSite()
	return nil
}

// ShouldQuit holds the window open until the UI has saved any typing
// (it calls ConfirmClose), or 3 seconds pass, so closing never loses text.
func ShouldQuit(a *App) bool {
	a.mu.Lock()
	allowed := a.closing
	a.closing = true
	a.mu.Unlock()
	if allowed {
		return true
	}
	application.Get().Event.Emit("app:closing")
	time.AfterFunc(3*time.Second, application.Get().Quit)
	return false
}

// ConfirmClose is called by the UI once pending edits are saved.
func (a *App) ConfirmClose() {
	a.mu.Lock()
	a.closing = true
	a.mu.Unlock()
	application.Get().Quit()
}

func (a *App) closeSite() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.preview.Stop()
	a.preview = nil
	if a.watcher != nil {
		a.watcher.Close()
		a.watcher = nil
	}
	a.site, a.repo, a.media, a.style, a.trial = nil, nil, nil, nil, nil
}

func (a *App) requireSite() (*site.Site, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.site == nil {
		return nil, &AppError{Code: "no-site", Message: "No site is open."}
	}
	return a.site, nil
}

type Welcome struct {
	Recent       []settings.RecentSite `json:"recent"`
	Name         string                `json:"name"`
	Email        string                `json:"email"`
	SitesFolder  string                `json:"sitesFolder"`
	GitHubSignIn bool                  `json:"githubSignIn"`
	Platform     string                `json:"platform"`
	Language     string                `json:"language"`
	Version      string                `json:"version"`
}

func (a *App) GetWelcome() Welcome {
	d := a.settings.Get()
	return Welcome{Recent: d.Recent, Name: d.Name, Email: d.Email, SitesFolder: d.SitesFolder,
		GitHubSignIn: a.github.ClientID != "", Platform: goos(), Language: d.Language, Version: version}
}

// CheckForUpdate returns the newer release to offer, or nil. GitHub is asked once per run.
func (a *App) CheckForUpdate() *update.Release {
	a.updateOnce.Do(func() {
		a.updateResult = update.Check(a.ctx, http.DefaultClient, update.LatestURL, version)
	})
	return a.updateResult
}

// SaveLanguage stores the language Pageroom is shown in ("" follows the system).
func (a *App) SaveLanguage(language string) error {
	return a.settings.Update(func(d *settings.Data) { d.Language = language })
}

func (a *App) SaveIdentity(name, email string) error {
	name, email = strings.TrimSpace(name), strings.TrimSpace(email)
	if name == "" {
		return &AppError{Code: "name-required", Message: "Please enter your name."}
	}
	return a.settings.Update(func(d *settings.Data) { d.Name, d.Email = name, email })
}

func (a *App) ChooseFolder(title string) (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle(title).
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(true).
		PromptForSingleSelection()
}

// ForgetSite removes a site from the recent list. Nothing is deleted.
func (a *App) ForgetSite(path string) error { return a.settings.Forget(path) }

func (a *App) OpenSite(path string) (*SiteState, error) {
	a.closeSite()
	trash := filepath.Join(a.dirs.Data, "trash", fsutil.Slugify(filepath.Base(path)))
	s, err := site.Open(path, trash)
	if err != nil {
		return nil, err
	}
	// A template that can't be updated leaves the site working with the older one.
	_, _ = s.UpdateShortcodes()
	var repo *gitsync.Repo
	if r, err := gitsync.Open(s.Root, a.gitOptions()); err == nil {
		repo = r
		a.secureRemote(repo)
	}
	staticDir := "static/images"
	if s.Prefs.Media.Folder != "" {
		staticDir = s.Prefs.Media.Folder
	}
	a.mu.Lock()
	a.site, a.repo = s, repo
	a.media = &media.Library{Root: s.Root, ContentDir: s.ContentDir, StaticDir: staticDir, MaxWidth: s.Prefs.Media.MaxWidth, TrashDir: trash}
	a.style = &appearance.Engine{Root: s.Root, Theme: s.Config.Theme(), HeadHook: s.Prefs.Appearance.HeadHook}
	a.mu.Unlock()
	if w, err := newWatcher(s.Root, func() { application.Get().Event.Emit("site:files-changed") }); err == nil {
		a.mu.Lock()
		a.watcher = w
		a.mu.Unlock()
	}
	remote := ""
	if repo != nil {
		remote = repo.RemoteURL()
	}
	// The recent-sites list is a convenience. Failing to update it must not block opening the site.
	_ = a.settings.Touch(settings.RecentSite{Path: s.Root, Title: s.Settings().Title, Remote: remote})
	return a.GetSite()
}

// secureRemote moves credentials embedded in the remote URL into the OS
// keychain, so a pasted token doesn't sit in .git/config.
func (a *App) secureRemote(repo *gitsync.Repo) {
	clean, user, pass, changed := secrets.StripURLCredentials(repo.RemoteURL())
	if !changed {
		return
	}
	if err := a.creds.Save(clean, secrets.Credential{Kind: "token", Username: user, Secret: pass}); err != nil {
		return // keep things working rather than lose the credential
	}
	// If this fails the URL keeps its credentials, but they are now also in the keychain, and the next open retries.
	_ = repo.SetRemote(clean)
}

func (a *App) CloneSite(remoteURL, folderName string) (*SiteState, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return nil, &AppError{Code: "repo-address-required", Message: "Please paste the repository's address."}
	}
	clean, user, pass, changed := secrets.StripURLCredentials(remoteURL)
	if changed {
		if err := a.creds.Save(clean, secrets.Credential{Kind: "token", Username: user, Secret: pass}); err != nil {
			return nil, err
		}
		remoteURL = clean
	}
	if folderName == "" {
		folderName = strings.TrimSuffix(filepath.Base(strings.TrimRight(remoteURL, "/")), ".git")
	}
	parent := a.settings.Get().SitesFolder
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	dest := fsutil.UniquePath(filepath.Join(parent, fsutil.Slugify(folderName)))
	application.Get().Event.Emit("busy", "Downloading your site…")
	if _, err := gitsync.Clone(a.ctx, remoteURL, dest, a.gitOptions()); err != nil {
		return nil, err
	}
	return a.OpenSite(dest)
}

func (a *App) CreateSite(title string) (*SiteState, error) {
	parent := a.settings.Get().SitesFolder
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	dest := fsutil.UniquePath(filepath.Join(parent, fsutil.Slugify(title)))
	if err := starter.Create(dest, title, hugo.DefaultVersion); err != nil {
		return nil, err
	}
	repo, err := gitsync.Init(dest, a.gitOptions())
	if err != nil {
		return nil, err
	}
	if err := repo.CommitAll("Create " + title); err != nil {
		return nil, err
	}
	return a.OpenSite(dest)
}

type SiteState struct {
	Info     site.Info       `json:"info"`
	Tree     *site.Node      `json:"tree"`
	Settings site.Settings   `json:"settings"`
	Git      GitState        `json:"git"`
	Hook     appearance.Hook `json:"styleHook"`
}

type GitState struct {
	IsRepo   bool   `json:"isRepo"`
	Remote   string `json:"remote"`
	Host     string `json:"host"`
	Branch   string `json:"branch"`
	SignedIn bool   `json:"signedIn"`
	// Published is false until the site first reaches the online repository.
	Published bool `json:"published"`
	// Pages is set when the repository is on GitHub.
	Pages *GitHubPages `json:"pages,omitempty"`
	// Cloudflare is set when the repository is on GitHub or GitLab.
	Cloudflare *Cloudflare `json:"cloudflare,omitempty"`
	// Direct is set when Pageroom uploads the site to Cloudflare itself.
	Direct *DirectUpload `json:"direct,omitempty"`
}

func (a *App) GetSite() (*SiteState, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	tree, err := s.Tree()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	style := a.style
	a.mu.Unlock()
	return &SiteState{Info: s.Info(), Tree: tree, Settings: s.Settings(), Git: a.gitState(), Hook: style.DetectHook()}, nil
}

func (a *App) gitState() GitState {
	a.mu.Lock()
	repo, s := a.repo, a.site
	a.mu.Unlock()
	if repo == nil || s == nil {
		return GitState{}
	}
	g := GitState{IsRepo: true, Remote: repo.RemoteURL(), Branch: repo.Branch(), Published: repo.Published()}
	g.Host = hostLabel(g.Remote)
	g.Direct = directUpload(repo)
	if g.Remote != "" {
		_, err := a.creds.Get(g.Remote)
		g.SignedIn = err == nil || isSSH(g.Remote)
		g.Pages = githubPages(g.Remote, s.Root)
		g.Cloudflare = cloudflare(g.Remote, s.Root)
	}
	return g
}

func hostLabel(remote string) string {
	k := strings.TrimPrefix(secrets.HostKey(remote), "git:")
	if i := strings.Index(k, "://"); i >= 0 {
		k = k[i+3:]
	}
	return k
}

func (a *App) GetTree() (*site.Node, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	return s.Tree()
}

func (a *App) RevealSite() error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	return openPath(s.Root)
}

func (a *App) OpenExternal(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return errors.New("only web links can be opened")
	}
	return application.Get().Browser.OpenURL(shellCharacterEncoder.Replace(url))
}

// Wails refuses to open links containing shell characters, and only logs it.
var shellCharacterEncoder = strings.NewReplacer(
	" ", "%20",
	"!", "%21",
	"$", "%24",
	"(", "%28",
	")", "%29",
	"*", "%2A",
	";", "%3B",
	"<", "%3C",
	">", "%3E",
	"[", "%5B",
	"\\", "%5C",
	"]", "%5D",
	"`", "%60",
	"{", "%7B",
	"|", "%7C",
	"}", "%7D",
	"~", "%7E",
)

// SiteFiles serves images and fonts from inside the open site.
func SiteFiles(a *App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/site-file/"
		if !strings.HasPrefix(r.URL.Path, prefix) || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, prefix)
		fontType := media.FontType(rel)
		if !media.IsImage(rel) && fontType == "" {
			http.NotFound(w, r)
			return
		}
		a.mu.Lock()
		s := a.site
		a.mu.Unlock()
		if s == nil {
			http.NotFound(w, r)
			return
		}
		full, err := fsutil.SafeJoin(s.Root, rel)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if fontType != "" {
			w.Header().Set("Content-Type", fontType)
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// SVGs are shown as images only. This stops any script inside one.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, full)
	})
}
