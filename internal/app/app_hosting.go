package app

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrheimb/pageroom/internal/fsutil"
	"github.com/myrheimb/pageroom/internal/hugo"
)

// GitHubPages is how a site kept on GitHub can be hosted there for free.
type GitHubPages struct {
	Address  string `json:"address"`  // where the live site appears
	Settings string `json:"settings"` // the repository's Pages settings on github.com
	SetUp    bool   `json:"setUp"`    // the site has a workflow that publishes to Pages
}

func githubRepo(remote string) (owner, name string, ok bool) {
	for _, prefix := range []string{"https://github.com/", "git@github.com:", "ssh://git@github.com/"} {
		if rest, found := strings.CutPrefix(remote, prefix); found {
			owner, name, _ = strings.Cut(strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git"), "/")
			return owner, name, owner != "" && name != "" && !strings.Contains(name, "/")
		}
	}
	return "", "", false
}

func githubPages(remote, root string) *GitHubPages {
	owner, name, ok := githubRepo(remote)
	if !ok {
		return nil
	}
	host := strings.ToLower(owner) + ".github.io"
	address := "https://" + host + "/" + name + "/"
	if strings.EqualFold(name, host) {
		address = "https://" + host + "/"
	}
	return &GitHubPages{Address: address, Settings: "https://github.com/" + owner + "/" + name + "/settings/pages", SetUp: hasPagesWorkflow(root)}
}

func hasPagesWorkflow(root string) bool {
	files, _ := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.y*ml"))
	for _, file := range files {
		if content, err := os.ReadFile(file); err == nil && bytes.Contains(content, []byte("actions/deploy-pages")) {
			return true
		}
	}
	return false
}

// pagesWorkflow builds with the same Hugo version as the preview. The
// address comes from GitHub, so links work on user.github.io/repo/ and on a
// custom domain.
const pagesWorkflow = `# Builds the site with Hugo and publishes it on GitHub Pages.
# Set up by Pageroom. In the repository's Settings, under Pages, set
# Source to "GitHub Actions".
name: Publish to GitHub Pages

on:
  push:
    branches: ["{{BRANCH}}"]
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: pages
  cancel-in-progress: false

jobs:
  build:
    runs-on: ubuntu-latest
    env:
      HUGO_VERSION: {{HUGO_VERSION}}
    steps:
      - name: Install Hugo
        run: |
          curl -fsSLo hugo.deb "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-amd64.deb"
          sudo dpkg -i hugo.deb
      - uses: actions/checkout@v7
        with:
          submodules: recursive
      - id: pages
        uses: actions/configure-pages@v6
      - name: Build
        run: hugo --gc --minify --baseURL "${{ steps.pages.outputs.base_url }}/"
      - uses: actions/upload-pages-artifact@v5
        with:
          path: ./public

  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - id: deployment
        uses: actions/deploy-pages@v5
`

// SetUpGitHubPages adds a workflow that publishes the site on GitHub Pages
// and points the site's address there unless it already has a real one.
func (a *App) SetUpGitHubPages() (*GitHubPages, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	repo, err := a.requireRepo()
	if err != nil {
		return nil, err
	}
	pages := githubPages(repo.RemoteURL(), s.Root)
	if pages == nil {
		return nil, &AppError{Code: "github-only", Message: "GitHub Pages only works for sites kept on GitHub."}
	}

	if !pages.SetUp {
		workflow := strings.NewReplacer("{{BRANCH}}", repo.Branch(), "{{HUGO_VERSION}}", hugo.SiteVersion(s.Root, s.Prefs.Hugo.Version)).Replace(pagesWorkflow)
		err := fsutil.CreateExclusive(filepath.Join(s.Root, ".github", "workflows", "pages.yml"), strings.NewReader(workflow))
		if errors.Is(err, os.ErrExist) {
			return nil, &AppError{Code: "pages-workflow-exists", Message: "This site already has a file called .github/workflows/pages.yml. Ask your developer to look at it."}
		}
		if err != nil {
			return nil, err
		}
		pages.SetUp = true
	}

	settings := s.Settings()
	if settings.BaseURL == "" || strings.Contains(settings.BaseURL, "example.org") || strings.Contains(settings.BaseURL, "example.com") {
		settings.BaseURL = pages.Address
		if err := s.SaveSettings(settings); err != nil {
			return nil, err
		}
		if err := s.Reload(); err != nil {
			return nil, err
		}
	}
	return pages, nil
}

// Cloudflare is how a site on GitHub or GitLab can be hosted on Cloudflare
// for free, as a Worker that serves the built site.
type Cloudflare struct {
	Name  string `json:"name"`  // the Worker's name, which must match on Cloudflare
	SetUp bool   `json:"setUp"` // the site has a Wrangler configuration
}

func cloudflare(remote, root string) *Cloudflare {
	if !strings.Contains(remote, "github.com") && !strings.Contains(remote, "gitlab.com") {
		return nil
	}
	name := fsutil.Slugify(strings.TrimSuffix(path.Base(strings.TrimSuffix(remote, "/")), ".git"))
	name = strings.Trim(name[:min(len(name), 63)], "-")
	setUp := false
	for _, file := range []string{"wrangler.jsonc", "wrangler.json", "wrangler.toml"} {
		setUp = setUp || fsutil.Exists(filepath.Join(root, file))
	}
	return &Cloudflare{Name: name, SetUp: setUp}
}

const cloudflareConfig = `// Tells Cloudflare how to build and serve the site. Set up by Pageroom.
{
  "name": "{{NAME}}",
  "compatibility_date": "{{DATE}}",
  "workers_dev": true,
  "preview_urls": false,
  "build": {
    "command": "chmod a+x build.sh && ./build.sh"
  },
  "assets": {
    "directory": "./public",
    "not_found_handling": "404-page"
  }
}
`

// cloudflareBuild uses the same Hugo version as the preview. Until the site
// has its real address, links work at any address. It skips minifying
// because the minifier fails on the invalid JSON-LD some themes (PaperMod)
// write in that case.
const cloudflareBuild = `#!/usr/bin/env bash
# Builds the site with Hugo on Cloudflare. Set up by Pageroom.
set -euo pipefail

HUGO_VERSION="{{HUGO_VERSION}}"

mkdir -p "${HOME}/.local/bin"
curl -fsSL "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-amd64.tar.gz" | tar -xz -C "${HOME}/.local/bin" hugo
export PATH="${HOME}/.local/bin:${PATH}"

if [ -f .gitmodules ]; then
  git submodule update --init --recursive
fi

if hugo config --format json | grep -qE '"baseurl": "(https?://example\.(org|com)/?)?"'; then
  hugo --gc --baseURL /
else
  hugo --gc --minify
fi
`

// SetUpCloudflare adds the files Cloudflare needs to build the site with
// Hugo and serve it each time someone publishes.
func (a *App) SetUpCloudflare() (*Cloudflare, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	repo, err := a.requireRepo()
	if err != nil {
		return nil, err
	}
	setup := cloudflare(repo.RemoteURL(), s.Root)
	if setup == nil {
		return nil, &AppError{Code: "cloudflare-host", Message: "Cloudflare can build sites kept on GitHub or GitLab."}
	}
	if setup.SetUp {
		return setup, nil
	}

	files := map[string]string{
		"wrangler.jsonc": strings.NewReplacer("{{NAME}}", setup.Name, "{{DATE}}", time.Now().Format(time.DateOnly)).Replace(cloudflareConfig),
		"build.sh":       strings.ReplaceAll(cloudflareBuild, "{{HUGO_VERSION}}", hugo.SiteVersion(s.Root, s.Prefs.Hugo.Version)),
	}
	if fsutil.Exists(filepath.Join(s.Root, "build.sh")) {
		return nil, &AppError{Code: "build-script-exists", Message: "This site already has a file called build.sh. Ask your developer to look at it."}
	}
	for name, content := range files {
		if err := fsutil.CreateExclusive(filepath.Join(s.Root, name), strings.NewReader(content)); err != nil {
			return nil, err
		}
	}
	setup.SetUp = true
	return setup, nil
}

// hostingFiles are the files Pageroom adds for each host, and the address
// suffix the host gives a site.
var hostingFiles = map[string]struct {
	files   []string
	address string
}{
	"github-pages": {[]string{".github/workflows/pages.yml"}, ".github.io"},
	"cloudflare":   {[]string{"wrangler.jsonc", "build.sh"}, ".workers.dev"},
}

// RemoveHosting removes what Pageroom added for a host, so the site can move
// to another one. Files a developer wrote stay, and so does a custom domain.
// The host itself is turned off on its own website.
func (a *App) RemoveHosting(host string) error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	repo, err := a.requireRepo()
	if err != nil {
		return err
	}
	hosting, ok := hostingFiles[host]
	if !ok {
		return &AppError{Code: "unknown-host", Message: "Pageroom doesn't know that host."}
	}

	for _, file := range hosting.files {
		content, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(file)))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !bytes.Contains(content, []byte("Set up by Pageroom")) {
			return &AppError{Code: "not-ours-file", Message: fmt.Sprintf("Pageroom didn't add %s, so it leaves it alone. Ask your developer to remove this hosting setup.", file), Details: map[string]string{"file": file}}
		}
	}
	for _, file := range hosting.files {
		if err := os.Remove(filepath.Join(s.Root, filepath.FromSlash(file))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	// Only removed when empty, so a developer's other workflows stay.
	_ = os.Remove(filepath.Join(s.Root, ".github", "workflows"))
	_ = os.Remove(filepath.Join(s.Root, ".github"))

	pages, worker := githubPages(repo.RemoteURL(), s.Root), cloudflare(repo.RemoteURL(), s.Root)
	if host == "github-pages" && pages != nil && pages.SetUp || host == "cloudflare" && worker != nil && worker.SetUp {
		return &AppError{Code: "not-ours", Message: "Your site has more hosting setup that Pageroom didn't add, so it leaves it alone. Ask your developer to remove it."}
	}

	settings := s.Settings()
	if address, err := url.Parse(settings.BaseURL); err == nil && strings.HasSuffix(address.Hostname(), hosting.address) {
		settings.BaseURL = ""
		if err := s.SaveSettings(settings); err != nil {
			return err
		}
		return s.Reload()
	}
	return nil
}
