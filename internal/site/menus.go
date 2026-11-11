package site

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/myrheimb/pageroom/internal/frontmatter"
	"github.com/myrheimb/pageroom/internal/fsutil"
)

type MenuItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	PageRef string `json:"pageRef,omitempty"` // a page on this site, e.g. "/about"
	URL     string `json:"url,omitempty"`     // an external link
	Weight  int    `json:"weight"`
	Parent  string `json:"parent,omitempty"` // ID of the parent item, for dropdowns
	Source  string `json:"source"`           // "config" or the page path that declares it
}

type Menu struct {
	Name     string     `json:"name"`
	Language string     `json:"language"`
	Items    []MenuItem `json:"items"`
}

// menuStore is where menu config is written: a split config file
// (config/_default/menus.<lang>.toml or menus.toml) or a key in the main
// config file.
type menuStore struct {
	rel  string
	doc  *frontmatter.Document
	key  string // "" when the file root is the menus map
	keys map[string]any
}

func (s *Site) menuStore(language string) (*menuStore, error) {
	names := []string{"menus"}
	multilingual := len(s.contentLanguages()) > 1
	if multilingual {
		names = []string{"menus." + language, "menus"}
	}
	for _, name := range names {
		for _, ext := range []string{".toml", ".yaml", ".yml", ".json"} {
			rel := "config/_default/" + name + ext
			b, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(rel)))
			if err != nil {
				continue
			}
			doc, err := frontmatter.ParseData(b, frontmatter.FormatFromExt(ext))
			if err != nil {
				return nil, err
			}
			st := &menuStore{rel: rel, doc: doc, keys: doc.Fields()}
			if name == "menus" && multilingual {
				return st.forLanguage(language)
			}
			return st, nil
		}
	}
	key := "menus"
	if _, ok := s.Config.doc.Get("menu"); ok {
		key = "menu"
	}
	m := s.Config.Map(key)
	if m == nil {
		m = map[string]any{}
	}
	st := &menuStore{rel: s.Config.Path, doc: s.Config.doc, key: key, keys: m}
	if multilingual {
		return st.forLanguage(language)
	}
	return st, nil
}

// forLanguage reads from this store but writes to the language's own file,
// which starts as a copy of the menus the language used until now.
func (st *menuStore) forLanguage(language string) (*menuStore, error) {
	doc, err := frontmatter.ParseData(nil, frontmatter.TOML)
	if err != nil {
		return nil, err
	}
	for name, entries := range st.keys {
		if err := doc.Set(name, entries, frontmatter.TOML); err != nil {
			return nil, err
		}
	}
	return &menuStore{rel: "config/_default/menus." + language + ".toml", doc: doc, keys: st.keys}, nil
}

// Menus returns every menu of every language, merging config entries and
// page front matter.
func (s *Site) Menus() ([]Menu, error) {
	tree, err := s.Tree()
	if err != nil {
		return nil, err
	}
	var out []Menu
	for _, language := range s.contentLanguages() {
		menus, err := s.languageMenus(language, tree)
		if err != nil {
			return nil, err
		}
		out = append(out, menus...)
	}
	return out, nil
}

func (s *Site) languageMenus(language string, tree *Node) ([]Menu, error) {
	st, err := s.menuStore(language)
	if err != nil {
		return nil, err
	}
	byName := map[string]*Menu{}
	get := func(name string) *Menu {
		if byName[name] == nil {
			byName[name] = &Menu{Name: name, Language: language}
		}
		return byName[name]
	}
	for name, raw := range st.keys {
		list, _ := raw.([]any)
		for i, e := range list {
			m, _ := e.(map[string]any)
			if m == nil {
				continue
			}
			it := MenuItem{Source: "config", Weight: toInt(m["weight"])}
			it.Name, _ = m["name"].(string)
			it.PageRef, _ = m["pageRef"].(string)
			it.URL, _ = m["url"].(string)
			it.Parent, _ = m["parent"].(string)
			it.ID, _ = m["identifier"].(string)
			if it.ID == "" {
				it.ID = fmt.Sprintf("%s-%d", fsutil.Slugify(it.Name), i)
			}
			get(name).Items = append(get(name).Items, it)
		}
	}
	for _, want := range s.Prefs.Menus {
		get(want)
	}
	if len(byName) == 0 {
		get("main")
	}

	// Pages can put themselves in menus via front matter.
	var visit func(n *Node)
	visit = func(n *Node) {
		for _, tr := range n.Translations {
			if tr.Language == language {
				s.pageMenus(tr.Path, n.Title, get)
			}
		}
		if len(n.Translations) == 0 && language == s.defaultLanguage() {
			file := n.Path
			if n.Kind == "section" {
				file = n.IndexMD
			}
			if file != "" {
				s.pageMenus(file, n.Title, get)
			}
		}
		for _, c := range n.Children {
			visit(c)
		}
	}
	visit(tree)

	var out []Menu
	for _, m := range byName {
		sort.SliceStable(m.Items, func(i, j int) bool { return m.Items[i].Weight < m.Items[j].Weight })
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Name == "main") != (out[j].Name == "main") {
			return out[i].Name == "main"
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *Site) pageMenus(file, title string, get func(string) *Menu) {
	p, err := s.GetPage(file)
	if err != nil || p.ParseError != "" {
		return
	}
	if own, _ := p.Fields["title"].(string); own != "" {
		title = own
	}
	ref := s.pageRef(file)
	for _, key := range []string{"menus", "menu"} {
		switch v := p.Fields[key].(type) {
		case string:
			get(v).Items = append(get(v).Items, MenuItem{ID: "page:" + file, Name: title, PageRef: ref, Source: file})
		case []any:
			for _, n := range v {
				if name, ok := n.(string); ok {
					get(name).Items = append(get(name).Items, MenuItem{ID: "page:" + file, Name: title, PageRef: ref, Source: file})
				}
			}
		case map[string]any:
			for name, cfg := range v {
				it := MenuItem{ID: "page:" + file, Name: title, PageRef: ref, Source: file}
				if c, ok := cfg.(map[string]any); ok {
					if n, _ := c["name"].(string); n != "" {
						it.Name = n
					}
					if id, _ := c["identifier"].(string); id != "" {
						it.ID = id
					}
					it.Parent, _ = c["parent"].(string)
					it.Weight = toInt(c["weight"])
				}
				get(name).Items = append(get(name).Items, it)
			}
		}
	}
}

// pageRef converts "content/posts/hi/index.md" to "/posts/hi".
func (s *Site) pageRef(rel string) string {
	r := strings.TrimPrefix(strings.TrimPrefix(rel, s.ContentDir), "/")
	dir, base := path.Split(r)
	stem, _ := s.splitLanguage(base)
	if stem == "index" || stem == "_index" {
		return "/" + strings.Trim(dir, "/")
	}
	return "/" + dir + stem
}

// SaveMenu stores the menu in the site config, in the given order, and
// removes that menu from page front matter so nothing is listed twice.
func (s *Site) SaveMenu(m Menu) error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("menu needs a name")
	}
	if m.Language == "" {
		m.Language = s.defaultLanguage()
	}
	if !slices.Contains(s.contentLanguages(), m.Language) {
		return fmt.Errorf("unknown language %q", m.Language)
	}
	st, err := s.menuStore(m.Language)
	if err != nil {
		return err
	}
	// Items that came from page front matter get a stable identifier so
	// dropdown children can point at them.
	ids := map[string]string{}
	used := map[string]bool{}
	for _, it := range m.Items {
		id := it.ID
		if id == "" || strings.HasPrefix(id, "page:") {
			id = fsutil.Slugify(it.Name)
		}
		for base, n := id, 2; used[id]; n++ {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		used[id] = true
		ids[it.ID] = id
	}
	existing := map[string]map[string]any{}
	oldList, _ := st.keys[m.Name].([]any)
	for _, raw := range oldList {
		if entry, _ := raw.(map[string]any); entry != nil {
			if id, _ := entry["identifier"].(string); id != "" {
				existing[id] = entry
			}
		}
	}
	list := make([]any, 0, len(m.Items))
	for i, it := range m.Items {
		e := map[string]any{}
		if kept, ok := existing[ids[it.ID]]; ok {
			e = kept
		}
		e["name"], e["weight"], e["identifier"] = it.Name, (i+1)*10, ids[it.ID]
		delete(e, "pageRef")
		delete(e, "url")
		delete(e, "parent")
		if it.PageRef != "" {
			e["pageRef"] = it.PageRef
		} else if it.URL != "" {
			e["url"] = it.URL
		}
		if it.Parent != "" {
			if p, ok := ids[it.Parent]; ok {
				e["parent"] = p
			} else {
				e["parent"] = it.Parent
			}
		}
		list = append(list, e)
	}

	// Pages that declared themselves in this menu: remove that declaration.
	for _, it := range m.Items {
		if it.Source != "" && it.Source != "config" {
			if err := s.removePageMenu(it.Source, m.Name); err != nil {
				return err
			}
		}
	}
	current, _ := s.Menus()
	for _, cm := range current {
		if cm.Name != m.Name || cm.Language != m.Language {
			continue
		}
		for _, it := range cm.Items {
			if it.Source != "config" {
				if err := s.removePageMenu(it.Source, m.Name); err != nil {
					return err
				}
			}
		}
	}

	if st.key == "" {
		if err := st.doc.Set(m.Name, list, st.doc.Format); err != nil {
			return err
		}
	} else {
		menus := st.keys
		menus[m.Name] = list
		if err := st.doc.Set(st.key, menus, st.doc.Format); err != nil {
			return err
		}
	}
	out, err := st.doc.Bytes()
	if err != nil {
		return err
	}
	full, err := s.abs(st.rel)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(full, out, 0o644)
}

func (s *Site) removePageMenu(file, menu string) error {
	full, err := s.abs(file)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return nil
	}
	doc, err := frontmatter.Parse(b)
	if err != nil {
		return nil
	}
	for _, key := range []string{"menus", "menu"} {
		v, ok := doc.Get(key)
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			if t == menu {
				doc.Delete(key)
			}
		case []any:
			var keep []any
			for _, n := range t {
				if n != menu {
					keep = append(keep, n)
				}
			}
			if len(keep) == 0 {
				doc.Delete(key)
			} else {
				doc.Set(key, keep, doc.Format)
			}
		case map[string]any:
			delete(t, menu)
			if len(t) == 0 {
				doc.Delete(key)
			} else {
				doc.Set(key, t, doc.Format)
			}
		}
	}
	out, err := doc.Bytes()
	if err != nil || bytes.Equal(out, b) {
		return err
	}
	return fsutil.WriteFileAtomic(full, out, 0o644)
}
