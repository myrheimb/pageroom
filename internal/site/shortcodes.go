package site

import (
	"embed"
	"path/filepath"

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
