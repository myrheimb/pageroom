// Package themes is the catalog behind the theme gallery: an index of themes
// from themes.gohugo.io that build with plain Hugo, and the safe unpacking of
// a theme download into a site.
package themes

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrheimb/pageroom/internal/fsutil"
)

// Index is the gallery's list of themes, built by cmd/themeindex.
type Index struct {
	HugoVersion string  `json:"hugoVersion"` // every theme was test-built with this Hugo
	Themes      []Theme `json:"themes"`
}

// Theme is one gallery entry, pinned to a commit so every download is
// exactly what was tested.
type Theme struct {
	Name           string    `json:"name"` // folder under themes/
	Title          string    `json:"title"`
	Description    string    `json:"description,omitempty"`
	Author         string    `json:"author,omitempty"`
	Tags           []string  `json:"tags,omitempty"`
	License        string    `json:"license"`
	Demo           string    `json:"demo,omitempty"`
	MinHugoVersion string    `json:"minHugoVersion,omitempty"`
	Updated        time.Time `json:"updated"`
	Module         string    `json:"module"`
	Commit         string    `json:"commit"`
	Archive        string    `json:"archive"`
	Subdir         string    `json:"subdir,omitempty"` // the theme's folder inside the archive
	SHA256         string    `json:"sha256"`           // of the unpacked files, see Extract
	Thumbnail      string    `json:"thumbnail"`
	Screenshot     string    `json:"screenshot,omitempty"`
}

//go:embed themes.json
var catalog []byte

// Catalog returns the gallery's themes, as of this build of Pageroom.
func Catalog() (Index, error) {
	var index Index
	err := json.Unmarshal(catalog, &index)
	return index, err
}

// MaxDownload caps the size of a theme download.
const MaxDownload = 100 << 20

// Install downloads theme into dir, which must not exist yet. The theme
// only appears in dir once the download matches what was tested.
func Install(ctx context.Context, client *http.Client, theme Theme, dir string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, theme.Archive, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the download failed: %s", response.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	// A hidden folder next to dir: the site's theme list skips it, and the
	// final rename stays on one disk.
	scratch, err := os.MkdirTemp(filepath.Dir(dir), ".pageroom-download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)

	unpacked := filepath.Join(scratch, "theme")
	sum, _, err := Extract(io.LimitReader(response.Body, MaxDownload), theme.Subdir, unpacked)
	if err != nil {
		return err
	}
	if sum != theme.SHA256 {
		return errors.New("the download doesn't match the tested version of the theme")
	}
	return os.Rename(unpacked, dir)
}

// MaxUnpacked caps a theme's unpacked size so a hostile archive can't fill the disk.
const MaxUnpacked = 200 << 20

// Extract unpacks a .tar.gz theme archive into dir, which must not exist yet.
// On error the caller removes dir. Only regular files under subdir are
// written, without the archive's top folder, the theme's exampleSite (Hugo
// never reads it) and links, which could point outside the site. It returns a
// hash of the written files, which unlike a hash of the archive survives a
// host recompressing it, and the commit time.
func Extract(r io.Reader, subdir, dir string) (sum string, updated time.Time, err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", time.Time{}, err
	}
	defer gz.Close()

	prefix := ""
	if subdir != "" {
		prefix = subdir + "/"
	}
	var files []string
	var total int64
	archive := tar.NewReader(gz)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", time.Time{}, err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		_, rel, _ := strings.Cut(header.Name, "/")
		rel, ok := strings.CutPrefix(rel, prefix)
		if !ok || rel == "exampleSite" || strings.HasPrefix(rel, "exampleSite/") {
			continue
		}
		if !filepath.IsLocal(filepath.FromSlash(rel)) {
			return "", time.Time{}, fmt.Errorf("the download contains an unsafe path: %q", header.Name)
		}

		total += header.Size
		if total > MaxUnpacked {
			return "", time.Time{}, fmt.Errorf("the theme is bigger than %d MB", MaxUnpacked>>20)
		}

		hash := sha256.New()
		if err := fsutil.CreateExclusive(filepath.Join(dir, filepath.FromSlash(rel)), io.TeeReader(archive, hash)); err != nil {
			return "", time.Time{}, err
		}
		files = append(files, rel+"\x00"+hex.EncodeToString(hash.Sum(nil)))
		if header.ModTime.After(updated) {
			updated = header.ModTime
		}
	}
	if len(files) == 0 {
		return "", time.Time{}, errors.New("the download has no theme files")
	}

	sort.Strings(files)
	whole := sha256.Sum256([]byte(strings.Join(files, "\n")))
	return hex.EncodeToString(whole[:]), updated.UTC(), nil
}
