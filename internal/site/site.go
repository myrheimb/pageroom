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
	"slices"
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
	Root     string `json:"root"`
	Title    string `json:"title"`
	BaseURL  string `json:"baseURL"`
	Theme    string `json:"theme"`
	Language string `json:"language"`
	// Languages are the content languages, the default first.
	Languages []string                `json:"languages"`
	Config    string                  `json:"config"`
	Prefs     *Prefs                  `json:"prefs"`
	Sections  map[string]SectionPrefs `json:"sections"`
}

func (s *Site) Info() Info {
	return Info{
		Root: s.Root, Title: s.Config.String("title"), BaseURL: s.Config.String("baseURL"),
		Theme: s.Config.Theme(), Language: s.languageValue(), Languages: s.contentLanguages(), Config: s.Config.Path,
		Prefs: s.Prefs, Sections: s.Prefs.Sections,
	}
}

func (s *Site) abs(rel string) (string, error) { return fsutil.SafeJoin(s.Root, rel) }

type Node struct {
	Path    string `json:"path"` // file for pages, directory for sections
	Kind    string `json:"kind"` // "section" or "page"
	Title   string `json:"title"`
	Draft   bool   `json:"draft"`
	Date    string `json:"date,omitempty"`
	Weight  int    `json:"weight"`
	Bundle  bool   `json:"bundle"`
	URL     string `json:"url"`
	IndexMD string `json:"indexFile,omitempty"` // section's _index.md if present
	// Translations lists the page in every language it has, the main one first.
	Translations []Translation `json:"translations,omitempty"`
	Children     []*Node       `json:"children,omitempty"`
	Error        string        `json:"error,omitempty"` // page could not be parsed
}

type Translation struct {
	Language string `json:"language"`
	Path     string `json:"path"`
	URL      string `json:"url"`
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
	var pages, indexes []*variant
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		rel := s.rel(full)
		if e.IsDir() {
			if files := s.indexFiles(full, "index"); len(files) > 0 {
				for _, file := range files {
					v := s.variant(file)
					v.node.Bundle = true
					v.nameKey = "d:" + name
					pages = append(pages, v)
				}
				continue
			}
			sec := &Node{Path: rel, Kind: "section", Title: humanize(name), URL: s.urlFor(rel + "/_index.md")}
			if err := s.walk(full, sec); err != nil {
				return err
			}
			parent.Children = append(parent.Children, sec)
			continue
		}
		if !contentExts[filepath.Ext(name)] {
			continue
		}
		v := s.variant(rel)
		stem, _ := s.splitLanguage(name)
		if stem == "_index" {
			indexes = append(indexes, v)
			continue
		}
		v.nameKey = "f:" + path.Join(path.Dir(rel), stem)
		pages = append(pages, v)
	}
	s.applyIndex(parent, indexes)
	parent.Children = append(parent.Children, s.groupPages(pages)...)
	sortNodes(parent.Children)
	return nil
}

// variant is one file of a page: the page in one language.
type variant struct {
	node     *Node
	lang     string
	nameKey  string // files with the same name apart from the language code are translations
	transKey string // Hugo's translationKey front matter, which wins over the name
}

func (s *Site) variant(rel string) *variant {
	node, key := s.pageNode(rel)
	_, lang := s.splitLanguage(path.Base(rel))
	return &variant{node: node, lang: lang, transKey: key}
}

// merge folds the variants of one page into the node of its main language.
func (s *Site) merge(variants []*variant) *Node {
	langs := s.contentLanguages()
	sort.SliceStable(variants, func(i, j int) bool {
		return slices.Index(langs, variants[i].lang) < slices.Index(langs, variants[j].lang)
	})
	node := variants[0].node
	for _, v := range variants {
		node.Translations = append(node.Translations, Translation{Language: v.lang, Path: v.node.Path, URL: v.node.URL})
	}
	return node
}

func (s *Site) groupPages(variants []*variant) []*Node {
	var order []string
	groups := map[string][]*variant{}
	for _, v := range variants {
		id := "n:" + v.nameKey
		if v.transKey != "" {
			id = "k:" + v.transKey
		}
		for n := 2; slices.ContainsFunc(groups[id], func(other *variant) bool { return other.lang == v.lang }); n++ {
			id = fmt.Sprintf("%s#%d", id, n)
		}
		if _, ok := groups[id]; !ok {
			order = append(order, id)
		}
		groups[id] = append(groups[id], v)
	}
	nodes := make([]*Node, 0, len(order))
	for _, id := range order {
		nodes = append(nodes, s.merge(groups[id]))
	}
	return nodes
}

// applyIndex takes the section's own title, weight and index file from its
// _index files, the default language first.
func (s *Site) applyIndex(sec *Node, indexes []*variant) {
	if len(indexes) == 0 {
		return
	}
	main := s.merge(indexes)
	sec.IndexMD = main.Path
	sec.Translations = main.Translations
	if main.Error == "" {
		if main.Title != "" && sec.Path != s.ContentDir {
			sec.Title = main.Title
		}
		sec.Weight, sec.Draft = main.Weight, main.Draft
	}
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

// indexFiles lists dir's index.md, index.<lang>.md and the like, for stem
// "index" or "_index".
func (s *Site) indexFiles(dir, stem string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !contentExts[path.Ext(name)] {
			continue
		}
		if base, _ := s.splitLanguage(name); base == stem {
			out = append(out, s.rel(filepath.Join(dir, name)))
		}
	}
	return out
}

func (s *Site) rel(full string) string {
	r, _ := filepath.Rel(s.Root, full)
	return filepath.ToSlash(r)
}

func (s *Site) pageNode(rel string) (*Node, string) {
	n := &Node{Path: rel, Kind: "page", URL: s.urlFor(rel)}
	full, _ := s.abs(rel)
	b, err := os.ReadFile(full)
	if err != nil {
		n.Error = err.Error()
		return n, ""
	}
	doc, err := frontmatter.Parse(b)
	if err != nil {
		n.Error = err.Error()
		n.Title = humanize(pageStem(s, rel))
		return n, ""
	}
	f := doc.Fields()
	n.Title, _ = f["title"].(string)
	if n.Title == "" {
		n.Title = humanize(pageStem(s, rel))
	}
	n.Draft, _ = f["draft"].(bool)
	n.Weight = toInt(f["weight"])
	n.Date = dateString(f["date"])
	if u := s.urlFromFields(rel, f); u != "" {
		n.URL = u
	}
	key, _ := f["translationKey"].(string)
	return n, key
}

func pageStem(s *Site, rel string) string {
	stem, _ := s.splitLanguage(path.Base(rel))
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
	stem, lang := s.splitLanguage(base)
	prefix := ""
	if lang != s.defaultLanguage() || s.defaultInSubdir() {
		prefix = "/" + lang
	}
	if stem == "index" || stem == "_index" {
		if dir == "" {
			return prefix + "/"
		}
		return prefix + "/" + strings.Trim(dir, "/") + "/"
	}
	return prefix + "/" + dir + stem + "/"
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
