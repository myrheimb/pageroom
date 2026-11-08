// Package site models a Hugo site the way a non-technical editor thinks
// about it: sections, pages, images and settings, rather than files.
package site

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrheimb/pageroom/internal/frontmatter"
	"github.com/myrheimb/pageroom/internal/fsutil"
)

type Site struct {
	Root       string
	ContentDir string // relative, usually "content"
	Config     *Config
	Prefs      *Prefs
	TrashDir   string // where deleted pages go before they are gone for good
}

func Open(dir, trashDir string) (*Site, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	rel, err := findConfig(abs)
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(abs, rel)
	if err != nil {
		return nil, fmt.Errorf("the site's settings file (%s) could not be read: %w", rel, err)
	}
	prefs, err := loadPrefs(abs)
	if err != nil {
		return nil, fmt.Errorf("%s could not be read: %w", PrefsFile, err)
	}
	content := cfg.String("contentDir")
	if content == "" {
		content = "content"
	}
	return &Site{Root: abs, ContentDir: path.Clean(content), Config: cfg, Prefs: prefs, TrashDir: trashDir}, nil
}

type Info struct {
	Root     string                  `json:"root"`
	Title    string                  `json:"title"`
	BaseURL  string                  `json:"baseURL"`
	Theme    string                  `json:"theme"`
	Language string                  `json:"language"`
	Config   string                  `json:"config"`
	Prefs    *Prefs                  `json:"prefs"`
	Sections map[string]SectionPrefs `json:"sections"`
}

func (s *Site) Info() Info {
	return Info{
		Root: s.Root, Title: s.Config.String("title"), BaseURL: s.Config.String("baseURL"),
		Theme: s.Config.Theme(), Language: s.languageValue(), Config: s.Config.Path,
		Prefs: s.Prefs, Sections: s.Prefs.Sections,
	}
}

func (s *Site) abs(rel string) (string, error) { return fsutil.SafeJoin(s.Root, rel) }

type Node struct {
	Path     string  `json:"path"` // file for pages, directory for sections
	Kind     string  `json:"kind"` // "section" or "page"
	Title    string  `json:"title"`
	Draft    bool    `json:"draft"`
	Date     string  `json:"date,omitempty"`
	Weight   int     `json:"weight"`
	Bundle   bool    `json:"bundle"`
	URL      string  `json:"url"`
	IndexMD  string  `json:"indexFile,omitempty"` // section's _index.md if present
	Children []*Node `json:"children,omitempty"`
	Error    string  `json:"error,omitempty"` // page could not be parsed
}

var contentExts = map[string]bool{".md": true, ".markdown": true, ".html": true}

func (s *Site) Tree() (*Node, error) {
	rootDir, err := s.abs(s.ContentDir)
	if err != nil {
		return nil, err
	}
	root := &Node{Path: s.ContentDir, Kind: "section", Title: "Home", URL: "/"}
	if _, err := os.Stat(rootDir); errors.Is(err, os.ErrNotExist) {
		return root, nil
	}
	if err := s.walk(rootDir, root); err != nil {
		return nil, err
	}
	return root, nil
}

func (s *Site) walk(dir string, parent *Node) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		rel := s.rel(full)
		if e.IsDir() {
			if idx := findIndex(full, "index"); idx != "" {
				n := s.pageNode(s.rel(idx))
				n.Bundle = true
				parent.Children = append(parent.Children, n)
				continue
			}
			sec := &Node{Path: rel, Kind: "section", Title: humanize(name), URL: s.urlFor(rel + "/_index.md")}
			if idx := findIndex(full, "_index"); idx != "" {
				sec.IndexMD = s.rel(idx)
				if p := s.pageNode(sec.IndexMD); p.Error == "" {
					if p.Title != "" {
						sec.Title = p.Title
					}
					sec.Weight, sec.Draft = p.Weight, p.Draft
				}
			}
			if err := s.walk(full, sec); err != nil {
				return err
			}
			parent.Children = append(parent.Children, sec)
			continue
		}
		if !contentExts[filepath.Ext(name)] || strings.HasPrefix(name, "_index.") {
			if strings.HasPrefix(name, "_index.") && parent.IndexMD == "" {
				parent.IndexMD = rel
			}
			continue
		}
		parent.Children = append(parent.Children, s.pageNode(rel))
	}
	sortNodes(parent.Children)
	return nil
}

// sortNodes orders like Hugo's default: weight, then date (newest first), then title.
func sortNodes(ns []*Node) {
	sort.SliceStable(ns, func(i, j int) bool {
		a, b := ns[i], ns[j]
		if (a.Weight == 0) != (b.Weight == 0) {
			return a.Weight != 0
		}
		if a.Weight != b.Weight {
			return a.Weight < b.Weight
		}
		if a.Kind != b.Kind {
			return a.Kind == "section"
		}
		if a.Date != b.Date {
			return a.Date > b.Date
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
}

func findIndex(dir, stem string) string {
	for ext := range contentExts {
		p := filepath.Join(dir, stem+ext)
		if fsutil.Exists(p) {
			return p
		}
	}
	return ""
}

func (s *Site) rel(full string) string {
	r, _ := filepath.Rel(s.Root, full)
	return filepath.ToSlash(r)
}

func (s *Site) pageNode(rel string) *Node {
	n := &Node{Path: rel, Kind: "page", URL: s.urlFor(rel)}
	full, _ := s.abs(rel)
	b, err := os.ReadFile(full)
	if err != nil {
		n.Error = err.Error()
		return n
	}
	doc, err := frontmatter.Parse(b)
	if err != nil {
		n.Error = err.Error()
		n.Title = humanize(pageStem(rel))
		return n
	}
	f := doc.Fields()
	n.Title, _ = f["title"].(string)
	if n.Title == "" {
		n.Title = humanize(pageStem(rel))
	}
	n.Draft, _ = f["draft"].(bool)
	n.Weight = toInt(f["weight"])
	n.Date = dateString(f["date"])
	if u := s.urlFromFields(rel, f); u != "" {
		n.URL = u
	}
	return n
}

func pageStem(rel string) string {
	base := path.Base(rel)
	stem := strings.TrimSuffix(base, path.Ext(base))
	if stem == "index" || stem == "_index" {
		return path.Base(path.Dir(rel))
	}
	return stem
}

// urlFor approximates Hugo's default permalink for a content file.
func (s *Site) urlFor(rel string) string {
	r := strings.TrimPrefix(rel, s.ContentDir)
	r = strings.TrimPrefix(r, "/")
	dir, base := path.Split(r)
	stem := strings.TrimSuffix(base, path.Ext(base))
	if stem == "index" || stem == "_index" {
		if dir == "" {
			return "/"
		}
		return "/" + strings.Trim(dir, "/") + "/"
	}
	return "/" + dir + stem + "/"
}

func (s *Site) urlFromFields(rel string, f map[string]any) string {
	if u, ok := f["url"].(string); ok && u != "" {
		return u
	}
	if slug, ok := f["slug"].(string); ok && slug != "" {
		u := s.urlFor(rel)
		parent := path.Dir(strings.TrimSuffix(u, "/"))
		if parent == "/" || parent == "." {
			return "/" + slug + "/"
		}
		return parent + "/" + slug + "/"
	}
	return ""
}

func humanize(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func toInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case uint64:
		return int(t)
	}
	return 0
}

func dateString(v any) string {
	switch t := v.(type) {
	case time.Time:
		return t.Format(time.RFC3339)
	case string:
		return t
	}
	return ""
}

// Hash identifies a version of file content, used to detect that a page
// changed on disk (a teammate's sync, another editor) since it was opened.
func Hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}
