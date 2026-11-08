// Package settings stores Pageroom's own preferences (recent sites, your
// name for publishing). Secrets are never stored here, see package secrets.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/myrheimb/pageroom/internal/fsutil"
)

type RecentSite struct {
	Path   string    `json:"path"`
	Title  string    `json:"title"`
	Opened time.Time `json:"opened"`
	Remote string    `json:"remote,omitempty"`
}

type Data struct {
	Name        string       `json:"name"`
	Email       string       `json:"email"`
	Recent      []RecentSite `json:"recent"`
	SitesFolder string       `json:"sitesFolder"` // where cloned sites go
	Theme       string       `json:"theme"`       // "light", "dark", "system"
	Language    string       `json:"language"`    // "en", "ja", or "" to follow the system
}

type Store struct {
	path string
	mu   sync.Mutex
	data Data
}

type Dirs struct {
	Config string
	Cache  string
	Data   string // trash, preview work dirs
}

func DefaultDirs() (Dirs, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return Dirs{}, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return Dirs{}, err
	}
	d := Dirs{
		Config: filepath.Join(cfg, "Pageroom"),
		Cache:  filepath.Join(cache, "Pageroom"),
		Data:   filepath.Join(cfg, "Pageroom", "data"),
	}
	for _, p := range []string{d.Config, d.Cache, d.Data} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return Dirs{}, err
		}
	}
	return d, nil
}

func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "settings.json")}
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &s.data)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if s.data.SitesFolder == "" {
		if home, err := os.UserHomeDir(); err == nil {
			s.data.SitesFolder = filepath.Join(home, "Pageroom Sites")
		}
	}
	return s, nil
}

func (s *Store) Get() Data {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.data
	d.Recent = append([]RecentSite(nil), s.data.Recent...)
	return d
}

func (s *Store) Update(fn func(*Data)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.data)
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(s.path, b, 0o600)
}

func (s *Store) Touch(site RecentSite) error {
	return s.Update(func(d *Data) {
		site.Opened = time.Now()
		out := []RecentSite{site}
		for _, r := range d.Recent {
			if r.Path != site.Path && fsutil.Exists(r.Path) {
				out = append(out, r)
			}
		}
		if len(out) > 12 {
			out = out[:12]
		}
		d.Recent = out
	})
}

// Forget removes a site from the recent list (files are untouched).
func (s *Store) Forget(path string) error {
	return s.Update(func(d *Data) {
		var out []RecentSite
		for _, r := range d.Recent {
			if r.Path != path {
				out = append(out, r)
			}
		}
		d.Recent = out
	})
}
