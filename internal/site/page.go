package site

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrheimb/pageroom/internal/frontmatter"
	"github.com/myrheimb/pageroom/internal/fsutil"
)

type Page struct {
	Path       string         `json:"path"`
	Hash       string         `json:"hash"`
	Format     string         `json:"format"`
	Keys       []string       `json:"keys"`
	Fields     map[string]any `json:"fields"`
	Body       string         `json:"body"`
	URL        string         `json:"url"`
	Bundle     bool           `json:"bundle"`
	IsSection  bool           `json:"isSection"`
	ParseError string         `json:"parseError,omitempty"`
}

// ChangedOnDiskError means the file changed since the editor loaded it.
// Saving anyway would overwrite someone else's work, so the UI asks.
type ChangedOnDiskError struct{ Path string }

func (e *ChangedOnDiskError) Error() string {
	return "This page was changed outside the editor (for example by getting a teammate's latest changes) since you opened it."
}

var dateKeys = map[string]bool{"date": true, "publishdate": true, "lastmod": true, "expirydate": true}

func (s *Site) GetPage(rel string) (*Page, error) {
	full, err := s.abs(rel)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	p := &Page{Path: rel, Hash: Hash(b), URL: s.urlFor(rel), Fields: map[string]any{}}
	base := path.Base(rel)
	p.Bundle = strings.HasPrefix(base, "index.")
	p.IsSection = strings.HasPrefix(base, "_index.")
	doc, err := frontmatter.Parse(b)
	if err != nil {
		// Show the raw file so the problem can be fixed instead of hiding the page.
		p.ParseError = err.Error()
		p.Body = string(b)
		return p, nil
	}
	p.Format = string(doc.Format)
	p.Keys = doc.Keys()
	for k, v := range doc.Fields() {
		if t, ok := v.(time.Time); ok {
			v = t.Format(time.RFC3339)
		}
		p.Fields[k] = v
	}
	if u := s.urlFromFields(rel, p.Fields); u != "" {
		p.URL = u
	}
	p.Body = string(doc.Body)
	return p, nil
}

// SaveRequest is a partial update: only what the user changed is sent.
type SaveRequest struct {
	Path     string         `json:"path"`
	BaseHash string         `json:"baseHash"`
	Set      map[string]any `json:"set"`
	Delete   []string       `json:"delete"`
	Body     *string        `json:"body"`
	Force    bool           `json:"force"` // user explicitly chose "keep my version"
}

// SavePage applies a SaveRequest, refusing if the file changed underneath.
func (s *Site) SavePage(req SaveRequest) (*Page, error) {
	full, err := s.abs(req.Path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	if !req.Force && req.BaseHash != "" && Hash(b) != req.BaseHash {
		return nil, &ChangedOnDiskError{Path: req.Path}
	}
	doc, err := frontmatter.Parse(b)
	if err != nil {
		return nil, err
	}
	fallback := s.DefaultFormat()
	for k, v := range req.Set {
		if err := doc.Set(k, coerce(k, v), fallback); err != nil {
			return nil, fmt.Errorf("could not save %q: %w", k, err)
		}
	}
	for _, k := range req.Delete {
		doc.Delete(k)
	}
	if req.Body != nil {
		body := *req.Body
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		doc.SetBody([]byte(body))
	}
	out, err := doc.Bytes()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(out, b) {
		if err := fsutil.WriteFileAtomic(full, out, 0o644); err != nil {
			return nil, err
		}
	}
	return s.GetPage(req.Path)
}

// coerce converts UI values (JSON strings and numbers) back into proper
// front matter types, so dates stay dates and whole numbers stay integers.
func coerce(key string, v any) any {
	switch t := v.(type) {
	case string:
		if dateKeys[strings.ToLower(key)] {
			for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
				if ts, err := time.Parse(layout, t); err == nil {
					if layout != time.RFC3339 {
						ts, _ = time.ParseInLocation(layout, t, time.Local)
					}
					return ts
				}
			}
		}
	case float64:
		if t == float64(int64(t)) {
			return int64(t)
		}
	case []any:
		for i := range t {
			t[i] = coerce("", t[i])
		}
	case map[string]any:
		for k := range t {
			t[k] = coerce(k, t[k])
		}
	}
	return v
}

// DefaultFormat picks the front matter syntax for new pages: whatever the
// default archetype uses, otherwise the config file's format.
func (s *Site) DefaultFormat() frontmatter.Format {
	for _, a := range []string{"archetypes/default.md", "themes/" + s.Config.Theme() + "/archetypes/default.md"} {
		if b, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(a))); err == nil {
			switch {
			case bytes.HasPrefix(b, []byte("+++")):
				return frontmatter.TOML
			case bytes.HasPrefix(b, []byte("---")):
				return frontmatter.YAML
			}
		}
	}
	if f := s.Config.Format(); f != frontmatter.None {
		return f
	}
	return frontmatter.YAML
}

type NewPage struct {
	Section string `json:"section"` // directory relative to site root
	Title   string `json:"title"`
	Bundle  bool   `json:"bundle"` // own folder, so images live next to the page
}

// CreatePage makes a new draft page and returns its path. It never
// overwrites: a taken name gets a numeric suffix.
func (s *Site) CreatePage(np NewPage) (string, error) {
	title := strings.TrimSpace(np.Title)
	if title == "" {
		return "", errors.New("please give the page a title")
	}
	section := np.Section
	if section == "" {
		section = s.ContentDir
	}
	if !s.inContent(section) {
		return "", fsutil.ErrOutsideRoot
	}
	dir, err := s.abs(section)
	if err != nil {
		return "", err
	}
	slug := fsutil.Slugify(title)
	var full string
	if np.Bundle {
		bundle := fsutil.UniquePath(filepath.Join(dir, slug))
		full = filepath.Join(bundle, "index.md")
	} else {
		full = fsutil.UniquePath(filepath.Join(dir, slug+".md"))
	}
	doc, _ := frontmatter.Parse(nil)
	f := s.DefaultFormat()
	doc.Set("title", title, f)
	doc.Set("date", time.Now().Truncate(time.Second), f)
	doc.Set("draft", true, f)
	out, err := doc.Bytes()
	if err != nil {
		return "", err
	}
	if err := fsutil.CreateExclusive(full, bytes.NewReader(out)); err != nil {
		return "", err
	}
	return s.rel(full), nil
}

func (s *Site) CreateSection(parent, title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", errors.New("please give the section a name")
	}
	if parent == "" {
		parent = s.ContentDir
	}
	if !s.inContent(parent) {
		return "", fsutil.ErrOutsideRoot
	}
	dir, err := s.abs(parent)
	if err != nil {
		return "", err
	}
	secDir := fsutil.UniquePath(filepath.Join(dir, fsutil.Slugify(title)))
	doc, _ := frontmatter.Parse(nil)
	doc.Set("title", title, s.DefaultFormat())
	out, _ := doc.Bytes()
	if err := fsutil.CreateExclusive(filepath.Join(secDir, "_index.md"), bytes.NewReader(out)); err != nil {
		return "", err
	}
	return s.rel(secDir), nil
}

func (s *Site) inContent(rel string) bool {
	c := path.Clean(rel)
	return c == s.ContentDir || strings.HasPrefix(c, s.ContentDir+"/")
}

// unit returns what physically moves with a page: its bundle folder for
// index.md, otherwise the file itself.
func unit(rel string) string {
	if strings.HasPrefix(path.Base(rel), "index.") {
		return path.Dir(rel)
	}
	return rel
}

// MovePage moves a page (and its images, for bundles) into another section,
// and records the old address in "aliases" so existing links keep working.
func (s *Site) MovePage(rel, toSection string) (string, error) {
	if !s.inContent(rel) || !s.inContent(toSection) {
		return "", fsutil.ErrOutsideRoot
	}
	oldURL := s.urlFor(rel)
	if p, err := s.GetPage(rel); err == nil {
		oldURL = p.URL
	}
	src, err := s.abs(unit(rel))
	if err != nil {
		return "", err
	}
	destDir, err := s.abs(toSection)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(destDir+string(filepath.Separator), src+string(filepath.Separator)) {
		return "", errors.New("a page can't be moved inside itself")
	}
	dest := fsutil.UniquePath(filepath.Join(destDir, filepath.Base(src)))
	if filepath.Dir(src) == destDir {
		return rel, nil
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(src, dest); err != nil {
		return "", err
	}
	newRel := s.rel(dest)
	if unit(rel) != rel {
		newRel = newRel + "/" + path.Base(rel)
	}
	return newRel, s.addAlias(newRel, oldURL)
}

// RenameSlug changes the page's web address, keeping the old one as an alias.
func (s *Site) RenameSlug(rel, newSlug string) (string, error) {
	newSlug = fsutil.Slugify(newSlug)
	p, err := s.GetPage(rel)
	if err != nil {
		return "", err
	}
	oldURL := p.URL
	src, err := s.abs(unit(rel))
	if err != nil {
		return "", err
	}
	name := newSlug
	if unit(rel) == rel {
		name += path.Ext(rel)
	}
	dest := filepath.Join(filepath.Dir(src), name)
	if dest == src {
		return rel, nil
	}
	dest = fsutil.UniquePath(dest)
	if err := os.Rename(src, dest); err != nil {
		return "", err
	}
	newRel := s.rel(dest)
	if unit(rel) != rel {
		newRel += "/" + path.Base(rel)
	}
	// A slug field would override the file name, so drop it.
	if _, ok := p.Fields["slug"]; ok {
		if _, err := s.SavePage(SaveRequest{Path: newRel, Delete: []string{"slug"}}); err != nil {
			return "", err
		}
	}
	return newRel, s.addAlias(newRel, oldURL)
}

func (s *Site) addAlias(rel, oldURL string) error {
	p, err := s.GetPage(rel)
	if err != nil || p.ParseError != "" || p.URL == oldURL || oldURL == "/" {
		return err
	}
	var aliases []any
	if a, ok := p.Fields["aliases"].([]any); ok {
		aliases = a
	}
	for _, a := range aliases {
		if a == oldURL {
			return nil
		}
	}
	aliases = append(aliases, oldURL)
	_, err = s.SavePage(SaveRequest{Path: rel, Set: map[string]any{"aliases": aliases}})
	return err
}

// DeletePage moves a page (or bundle, or whole section) to Pageroom's trash
// rather than deleting it, and returns where it went.
func (s *Site) DeletePage(rel string) (string, error) {
	if !s.inContent(rel) || path.Clean(rel) == s.ContentDir {
		return "", errors.New("that can't be deleted")
	}
	src, err := s.abs(unit(rel))
	if err != nil {
		return "", err
	}
	if s.TrashDir == "" {
		return "", errors.New("trash folder is not configured")
	}
	dest := filepath.Join(s.TrashDir, time.Now().Format("20060102-150405"), filepath.FromSlash(unit(rel)))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(src, dest); err != nil {
		// different volume: copy, then remove
		if err := copyTree(src, dest); err != nil {
			return "", err
		}
		if err := os.RemoveAll(src); err != nil {
			return "", err
		}
	}
	return dest, nil
}

func copyTree(src, dest string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		r, _ := filepath.Rel(src, p)
		target := filepath.Join(dest, r)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		return fsutil.CreateExclusive(target, f)
	})
}

// Reorder sets weights so pages appear in the given order (10, 20, 30... to
// leave room for manual tweaks). Only pages whose weight changes are written.
func (s *Site) Reorder(paths []string) error {
	for i, rel := range paths {
		if st, err := os.Stat(filepath.Join(s.Root, filepath.FromSlash(rel))); err == nil && st.IsDir() {
			idx := s.sectionIndex(rel)
			if idx == "" {
				// a folder without settings gets an _index.md to hold its position
				doc, _ := frontmatter.Parse(nil)
				doc.Set("title", humanize(path.Base(rel)), s.DefaultFormat())
				out, _ := doc.Bytes()
				idx = rel + "/_index.md"
				full, err := s.abs(idx)
				if err != nil {
					return err
				}
				if err := fsutil.CreateExclusive(full, bytes.NewReader(out)); err != nil {
					return err
				}
			}
			rel = idx
		}
		p, err := s.GetPage(rel)
		if err != nil {
			return err
		}
		if p.ParseError != "" {
			continue
		}
		want := (i + 1) * 10
		if toInt(p.Fields["weight"]) == want {
			continue
		}
		if _, err := s.SavePage(SaveRequest{Path: rel, BaseHash: p.Hash, Set: map[string]any{"weight": want}}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Site) sectionIndex(dir string) string {
	full, err := s.abs(dir)
	if err != nil {
		return ""
	}
	var variants []*variant
	for _, file := range s.indexFiles(full, "_index") {
		variants = append(variants, s.variant(file))
	}
	if len(variants) == 0 {
		return ""
	}
	return s.merge(variants).Path
}

// WriteRaw replaces a page file entirely (used to restore an older version),
// with the same protection against overwriting changes made on disk.
func (s *Site) WriteRaw(rel, baseHash string, content []byte) (*Page, error) {
	if !s.inContent(rel) {
		return nil, fsutil.ErrOutsideRoot
	}
	full, err := s.abs(rel)
	if err != nil {
		return nil, err
	}
	if cur, err := os.ReadFile(full); err == nil && baseHash != "" && Hash(cur) != baseHash {
		return nil, &ChangedOnDiskError{Path: rel}
	}
	if err := fsutil.WriteFileAtomic(full, content, 0o644); err != nil {
		return nil, err
	}
	return s.GetPage(rel)
}
