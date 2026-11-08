// Package starter creates a new Hugo site with Pageroom's starter theme, for
// people who don't have a site yet, such as when moving from WordPress.
package starter

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrheimb/pageroom/internal/frontmatter"
	"github.com/myrheimb/pageroom/internal/fsutil"
)

//go:embed all:files
var files embed.FS

// Create writes a new site into dir, which must be empty or not exist.
func Create(dir, title, hugoVersion string) error { return create(dir, title, hugoVersion, true) }

// CreateEmpty is Create without the sample posts and pages, for sites that
// are about to receive imported content.
func CreateEmpty(dir, title, hugoVersion string) error { return create(dir, title, hugoVersion, false) }

var samples = map[string]bool{"content/posts/welcome/index.md": true, "content/about/index.md": true}

func create(dir, title, hugoVersion string, withSamples bool) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return errors.New("that folder isn't empty; please choose an empty or new folder")
	}
	root, _ := fs.Sub(files, "files")
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !withSamples && samples[p] {
			return nil
		}
		b, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		b = bytes.ReplaceAll(b, []byte("{{HUGO_VERSION}}"), []byte(hugoVersion))
		return fsutil.CreateExclusive(filepath.Join(dir, filepath.FromSlash(p)), bytes.NewReader(b))
	})
	if err != nil {
		return err
	}
	// Set the title through the TOML editor so quotes in names are escaped.
	cfgPath := filepath.Join(dir, "hugo.toml")
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	doc, err := frontmatter.ParseData(b, frontmatter.TOML)
	if err != nil {
		return err
	}
	if strings.TrimSpace(title) == "" {
		title = "My website"
	}
	if err := doc.Set("title", title, frontmatter.TOML); err != nil {
		return err
	}
	out, err := doc.Bytes()
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(cfgPath, out, 0o644)
}
