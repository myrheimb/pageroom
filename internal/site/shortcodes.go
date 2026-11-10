package site

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/myrheimb/pageroom/internal/fsutil"
)

//go:embed shortcodes
var shortcodes embed.FS

// AddShortcode puts one of Pageroom's shortcodes into the site, unless the
// site already has its own with that name.
func (s *Site) AddShortcode(name string) error {
	full := filepath.Join(s.Root, "layouts", "shortcodes", name+".html")
	if fsutil.Exists(full) {
		return nil
	}
	data, err := shortcodes.ReadFile("shortcodes/" + name + ".html")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(full, data, 0o644)
}

// UpdateShortcodes replaces each Pageroom template in the site that still
// matches a version Pageroom shipped with the current one. Edited and missing
// files are left alone. It returns the names it replaced.
func (s *Site) UpdateShortcodes() ([]string, error) {
	var updated []string
	for name, hashes := range shippedShortcodeHashes {
		full := filepath.Join(s.Root, "layouts", "shortcodes", name+".html")
		existing, err := os.ReadFile(full)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return updated, err
		}
		current, err := shortcodes.ReadFile("shortcodes/" + name + ".html")
		if err != nil {
			return updated, err
		}
		if bytes.Equal(existing, current) || !slices.Contains(hashes, hashOf(existing)) {
			continue
		}
		if err := fsutil.WriteFileAtomic(full, current, 0o644); err != nil {
			return updated, err
		}
		updated = append(updated, name)
	}
	sort.Strings(updated)
	return updated, nil
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
