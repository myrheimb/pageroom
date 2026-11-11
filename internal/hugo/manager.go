// Package hugo finds or downloads the right Hugo binary for a site and runs
// it, so users never install anything by hand.
package hugo

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/myrheimb/pageroom/internal/fsutil"
)

// DefaultVersion is used when a site doesn't say which Hugo it needs.
const DefaultVersion = "0.168.0"

// Manager resolves Hugo binaries, caching downloads per version.
type Manager struct {
	CacheDir    string // e.g. ~/Library/Caches/Pageroom/hugo
	DownloadURL string // base, default GitHub releases
	HTTP        *http.Client
	Progress    func(done, total int64)
}

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// SiteVersion works out which Hugo version a site expects, from (in order)
// .pageroom.yaml, netlify.toml, a GitHub Actions workflow, or .hugo-version.
func SiteVersion(root, prefsVersion string) string {
	if v := strings.TrimPrefix(strings.TrimSpace(prefsVersion), "v"); versionRe.MatchString(v) {
		return v
	}
	patterns := []struct {
		file string
		re   *regexp.Regexp
	}{
		{"netlify.toml", regexp.MustCompile(`HUGO_VERSION\s*=\s*["']v?(\d+\.\d+\.\d+)["']`)},
		{".hugo-version", regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)},
		{".tool-versions", regexp.MustCompile(`hugo(?:-extended)?\s+(?:extended_)?v?(\d+\.\d+\.\d+)`)},
		{"wrangler.toml", regexp.MustCompile(`HUGO_VERSION\s*=\s*["']v?(\d+\.\d+\.\d+)["']`)},
	}
	for _, p := range patterns {
		if b, err := os.ReadFile(filepath.Join(root, p.file)); err == nil {
			if m := p.re.FindSubmatch(b); m != nil {
				return string(m[1])
			}
		}
	}
	wf := filepath.Join(root, ".github", "workflows")
	if entries, err := os.ReadDir(wf); err == nil {
		re := regexp.MustCompile(`HUGO_VERSION:\s*["']?v?(\d+\.\d+\.\d+)`)
		for _, e := range entries {
			if b, err := os.ReadFile(filepath.Join(wf, e.Name())); err == nil {
				if m := re.FindSubmatch(b); m != nil {
					return string(m[1])
				}
			}
		}
	}
	return DefaultVersion
}

func binName() string {
	if runtime.GOOS == "windows" {
		return "hugo.exe"
	}
	return "hugo"
}

// Choice is the Hugo Pageroom will use for a site, or what's missing.
type Choice struct {
	Path    string `json:"path,omitempty"`   // ready to run. Empty means a download is needed.
	Version string `json:"version"`          // what the site needs
	Source  string `json:"source,omitempty"` // "installed" or "pageroom"
	// Installed is the user's own Hugo when it couldn't be used because it
	// is a different version (or lacks the "extended" features).
	Installed *Installed `json:"installed,omitempty"`
}

// Choose picks a Hugo without downloading: the user's installed Hugo if it's
// the right version, else Pageroom's private copy if there is one. Path is
// empty when a download is needed.
func (m *Manager) Choose(version string, installed []Installed) Choice {
	c := Choice{Version: version}
	for _, in := range installed {
		if in.Version == version && in.Extended {
			c.Path, c.Source = in.Path, "installed"
			return c
		}
	}
	for i := range installed {
		if c.Installed == nil || installed[i].Extended {
			c.Installed = &installed[i]
		}
	}
	if cached := m.CachedPath(version); fsutil.Exists(cached) {
		c.Path, c.Source = cached, "pageroom"
	}
	return c
}

// CachedPath is where Pageroom keeps its private copy of a Hugo version.
// It's never put on the system PATH or installed anywhere else.
func (m *Manager) CachedPath(version string) string {
	return filepath.Join(m.CacheDir, version, binName())
}

// Download fetches Hugo into Pageroom's private cache. Call it only after
// the user agreed.
func (m *Manager) Download(ctx context.Context, version string) (string, error) {
	dest := m.CachedPath(version)
	if fsutil.Exists(dest) {
		return dest, nil
	}
	if err := m.download(ctx, version, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func (m *Manager) client() *http.Client {
	if m.HTTP != nil {
		return m.HTTP
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func (m *Manager) base(version string) string {
	if m.DownloadURL != "" {
		return strings.TrimRight(m.DownloadURL, "/") + "/v" + version
	}
	return "https://github.com/gohugoio/hugo/releases/download/v" + version
}

func (m *Manager) get(ctx context.Context, url string, limit int64, progress bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't download Hugo (are you online?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("couldn't download Hugo: %s returned HTTP %d", url, resp.StatusCode)
	}
	var r io.Reader = io.LimitReader(resp.Body, limit)
	if progress && m.Progress != nil {
		r = &progressReader{r: r, total: resp.ContentLength, cb: m.Progress}
	}
	return io.ReadAll(r)
}

// pickAsset chooses the archive for this OS/arch from the release's
// checksum list, preferring the "extended" edition most themes need.
func pickAsset(checksums, version, goos, goarch string) (name, sum string, err error) {
	osName := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "windows", "freebsd": "freebsd"}[goos]
	arches := []string{goarch}
	if goos == "darwin" {
		arches = []string{"universal", goarch}
	}
	type entry struct{ name, sum string }
	var entries []entry
	sc := bufio.NewScanner(strings.NewReader(checksums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 {
			entries = append(entries, entry{name: f[1], sum: f[0]})
		}
	}
	for _, edition := range []string{"hugo_extended_", "hugo_extended_withdeploy_", "hugo_"} {
		for _, arch := range arches {
			// macOS releases ship only as .pkg installers. Pageroom reads the
			// hugo program out of the package and never runs the installer.
			for _, ext := range []string{".tar.gz", ".zip", ".pkg"} {
				want := fmt.Sprintf("%s%s_%s-%s%s", edition, version, osName, arch, ext)
				for _, e := range entries {
					if e.name == want {
						return e.name, e.sum, nil
					}
				}
			}
		}
	}
	return "", "", fmt.Errorf("no Hugo %s download for %s/%s; please install Hugo yourself (gohugo.io/installation)", version, goos, goarch)
}

func (m *Manager) download(ctx context.Context, version, dest string) error {
	if !versionRe.MatchString(version) {
		return fmt.Errorf("invalid Hugo version %q", version)
	}
	sums, err := m.get(ctx, fmt.Sprintf("%s/hugo_%s_checksums.txt", m.base(version), version), 1<<20, false)
	if err != nil {
		return err
	}
	name, sum, err := pickAsset(string(sums), version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	archive, err := m.get(ctx, m.base(version)+"/"+name, 300<<20, true)
	if err != nil {
		return err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != strings.ToLower(sum) {
		return errors.New("the downloaded Hugo didn't match its published checksum and was discarded")
	}
	bin, err := extract(archive, name)
	if err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(dest, bin, 0o755); err != nil {
		return err
	}
	return os.Chmod(dest, 0o755)
}

// extract pulls only the hugo executable out of the archive, by exact
// name, so a malicious archive can't write anywhere else.
func extract(archive []byte, name string) ([]byte, error) {
	want := binName()
	if strings.HasSuffix(name, ".pkg") {
		return extractPkg(archive)
	}
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name == want {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer func() { _ = rc.Close() }()
				return io.ReadAll(io.LimitReader(rc, 500<<20))
			}
		}
		return nil, errors.New("hugo executable not found in download")
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("hugo executable not found in download")
		}
		if err != nil {
			return nil, err
		}
		if h.Name == want && h.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, 500<<20))
		}
	}
}

type progressReader struct {
	r     io.Reader
	done  int64
	total int64
	cb    func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	p.cb(p.done, p.total)
	return n, err
}
