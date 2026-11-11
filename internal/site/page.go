package site

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
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
	// Language is the page's content language, empty for the site's default.
	Language string `json:"language"`
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
	code, err := s.languageSuffix(np.Language)
	if err != nil {
		return "", err
	}
	slug := fsutil.Slugify(title)
	var full string
	if np.Bundle {
		bundle := fsutil.UniquePath(filepath.Join(dir, slug))
		full = filepath.Join(bundle, "index"+code+".md")
	} else {
		full = s.uniqueFile(dir, slug+code+".md")
	}
	doc, _ := frontmatter.Parse(nil)
	f := s.DefaultFormat()
	for _, field := range []struct {
		key   string
		value any
	}{
		{"title", title},
		{"date", time.Now().Truncate(time.Second)},
		{"draft", true},
	} {
		if err := doc.Set(field.key, field.value, f); err != nil {
			return "", err
		}
	}
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
	if err := doc.Set("title", title, s.DefaultFormat()); err != nil {
		return "", err
	}
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

// languageSuffix is the ".ja" part of a file name. The default language
// needs none.
func (s *Site) languageSuffix(lang string) (string, error) {
	if lang == "" || lang == s.defaultLanguage() {
		return "", nil
	}
	if !slices.Contains(s.contentLanguages(), lang) {
		return "", fmt.Errorf("%q is not one of the site's languages", lang)
	}
	return "." + lang, nil
}

// freeStem returns stem, or stem with "-2", "-3"... added, so that none of
// dir/<stem><suffix> exists. Translations share one stem, so the pair stays
// together.
func freeStem(dir, stem string, suffixes []string) string {
	candidate := stem
	for i := 2; slices.ContainsFunc(suffixes, func(suffix string) bool {
		return fsutil.Exists(filepath.Join(dir, candidate+suffix))
	}); i++ {
		candidate = fmt.Sprintf("%s-%d", stem, i)
	}
	return candidate
}

// uniqueFile returns dir/name, or the name with "-2", "-3"... added before
// the language code and extension when it is taken.
func (s *Site) uniqueFile(dir, name string) string {
	stem, _ := s.splitLanguage(name)
	suffix := s.fileSuffix(name)
	return filepath.Join(dir, freeStem(dir, stem, []string{suffix})+suffix)
}

// fileSuffix is the language code and extension of a page file name.
func (s *Site) fileSuffix(name string) string {
	stem, _ := s.splitLanguage(name)
	return strings.TrimPrefix(name, stem)
}

// translationIndex maps the path of every page file to the paths of all
// files of that page, so an operation can act on each translation.
func (s *Site) translationIndex() map[string][]string {
	index := map[string][]string{}
	tree, err := s.Tree()
	if err != nil {
		return index
	}
	var visit func(node *Node)
	visit = func(node *Node) {
		var paths []string
		for _, translation := range node.Translations {
			paths = append(paths, translation.Path)
		}
		for _, filePath := range paths {
			index[filePath] = paths
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(tree)
	return index
}

func pagePaths(index map[string][]string, rel string) []string {
	if paths, ok := index[rel]; ok {
		return paths
	}
	return []string{rel}
}

// MovePage moves a page, every translation and its images for bundles, into
// another section, and records the old addresses in "aliases" so existing
// links keep working. It returns the new path of rel.
func (s *Site) MovePage(rel, toSection string) (string, error) {
	if !s.inContent(rel) || !s.inContent(toSection) {
		return "", fsutil.ErrOutsideRoot
	}
	destDir, err := s.abs(toSection)
	if err != nil {
		return "", err
	}
	paths := pagePaths(s.translationIndex(), rel)
	oldURLs := map[string]string{}
	for _, filePath := range paths {
		oldURLs[filePath] = s.urlFor(filePath)
		if p, err := s.GetPage(filePath); err == nil {
			oldURLs[filePath] = p.URL
		}
		src, err := s.abs(unit(filePath))
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(destDir+string(filepath.Separator), src+string(filepath.Separator)) {
			return "", errors.New("a page can't be moved inside itself")
		}
		if filepath.Dir(src) == destDir {
			return rel, nil
		}
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	suffixesByStem := map[string][]string{}
	for _, filePath := range paths {
		unitPath := unit(filePath)
		name := path.Base(unitPath)
		stem, suffix := name, ""
		if unitPath == filePath {
			stem, _ = s.splitLanguage(name)
			suffix = s.fileSuffix(name)
		}
		suffixesByStem[stem] = append(suffixesByStem[stem], suffix)
	}
	newStems := map[string]string{}
	for stem, suffixes := range suffixesByStem {
		newStems[stem] = freeStem(destDir, stem, suffixes)
	}
	newPaths := map[string]string{}
	movedUnits := map[string]string{}
	for _, filePath := range paths {
		unitPath := unit(filePath)
		dest, moved := movedUnits[unitPath]
		if !moved {
			src, _ := s.abs(unitPath)
			name := path.Base(unitPath)
			if unitPath == filePath {
				stem, _ := s.splitLanguage(name)
				dest = filepath.Join(destDir, newStems[stem]+s.fileSuffix(name))
			} else {
				dest = filepath.Join(destDir, newStems[name])
			}
			if err := os.Rename(src, dest); err != nil {
				return "", err
			}
			movedUnits[unitPath] = dest
		}
		newPaths[filePath] = s.rel(dest)
		if unitPath != filePath {
			newPaths[filePath] += "/" + path.Base(filePath)
		}
	}
	for _, filePath := range paths {
		if err := s.addAlias(newPaths[filePath], oldURLs[filePath]); err != nil {
			return "", err
		}
	}
	return newPaths[rel], nil
}

// RenameSlug changes the page's web address on every translation, keeping
// each old address as an alias. A file keeps its language code. Fields that
// would override the new name are dropped. It returns the new path of rel.
func (s *Site) RenameSlug(rel, newSlug string) (string, error) {
	newSlug = fsutil.Slugify(newSlug)
	paths := pagePaths(s.translationIndex(), rel)
	oldURLs := map[string]string{}
	pageFields := map[string]map[string]any{}
	for _, filePath := range paths {
		p, err := s.GetPage(filePath)
		if err != nil {
			return "", err
		}
		oldURLs[filePath], pageFields[filePath] = p.URL, p.Fields
	}
	newPaths := map[string]string{}
	if unit(rel) != rel {
		src, err := s.abs(unit(rel))
		if err != nil {
			return "", err
		}
		dest := filepath.Join(filepath.Dir(src), newSlug)
		if dest == src {
			return rel, nil
		}
		dest = fsutil.UniquePath(dest)
		if err := os.Rename(src, dest); err != nil {
			return "", err
		}
		for _, filePath := range paths {
			newPaths[filePath] = s.rel(dest) + "/" + path.Base(filePath)
		}
	} else {
		src, err := s.abs(rel)
		if err != nil {
			return "", err
		}
		dir := filepath.Dir(src)
		first, _ := s.abs(paths[0])
		if filepath.Join(dir, newSlug+s.fileSuffix(filepath.Base(first))) == first {
			return rel, nil
		}
		suffixes := make([]string, len(paths))
		for i, filePath := range paths {
			suffixes[i] = s.fileSuffix(path.Base(filePath))
		}
		stem := freeStem(dir, newSlug, suffixes)
		for i, filePath := range paths {
			from, _ := s.abs(filePath)
			dest := filepath.Join(dir, stem+suffixes[i])
			if err := os.Rename(from, dest); err != nil {
				return "", err
			}
			newPaths[filePath] = s.rel(dest)
		}
	}
	for _, filePath := range paths {
		newRel := newPaths[filePath]
		var overrides []string
		for _, key := range []string{"slug", "url"} {
			if _, ok := pageFields[filePath][key]; ok {
				overrides = append(overrides, key)
			}
		}
		if len(overrides) > 0 {
			if _, err := s.SavePage(SaveRequest{Path: newRel, Delete: overrides}); err != nil {
				return "", err
			}
		}
		if err := s.addAlias(newRel, oldURLs[filePath]); err != nil {
			return "", err
		}
	}
	return newPaths[rel], nil
}

// AddTranslation creates the page in another language, next to the file of
// the page's main language, and returns its path. The new page copies the
// title as a starting point and is a draft.
func (s *Site) AddTranslation(rel, lang string) (string, error) {
	if !s.inContent(rel) {
		return "", fsutil.ErrOutsideRoot
	}
	code, err := s.languageSuffix(lang)
	if err != nil {
		return "", err
	}
	full, err := s.abs(rel)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(full); err == nil && info.IsDir() {
		if rel = s.sectionIndex(rel); rel == "" {
			return "", errors.New("that section has no settings page to translate")
		}
	}
	if lang == "" {
		lang = s.defaultLanguage()
	}
	paths := pagePaths(s.translationIndex(), rel)
	for _, filePath := range paths {
		if _, existing := s.splitLanguage(path.Base(filePath)); existing == lang {
			return "", errors.New("this page already has that language")
		}
	}
	source, err := s.GetPage(paths[0])
	if err != nil {
		return "", err
	}
	if source.ParseError != "" {
		return "", errors.New("the page can't be read, so it can't be translated")
	}
	stem, _ := s.splitLanguage(path.Base(paths[0]))
	newRel := path.Join(path.Dir(paths[0]), stem+code+path.Ext(paths[0]))
	target, err := s.abs(newRel)
	if err != nil {
		return "", err
	}
	doc, _ := frontmatter.Parse(nil)
	format := s.DefaultFormat()
	title, _ := source.Fields["title"].(string)
	if err := doc.Set("title", title, format); err != nil {
		return "", err
	}
	if key, ok := source.Fields["translationKey"].(string); ok && key != "" {
		if err := doc.Set("translationKey", key, format); err != nil {
			return "", err
		}
	}
	if !source.IsSection {
		if err := doc.Set("date", time.Now().Truncate(time.Second), format); err != nil {
			return "", err
		}
		if err := doc.Set("draft", true, format); err != nil {
			return "", err
		}
	}
	out, err := doc.Bytes()
	if err != nil {
		return "", err
	}
	if err := fsutil.CreateExclusive(target, bytes.NewReader(out)); err != nil {
		return "", err
	}
	return newRel, nil
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

// DeletePage moves a page (every translation, or bundle, or whole section)
// to Pageroom's trash rather than deleting it, and returns where rel went.
func (s *Site) DeletePage(rel string) (string, error) {
	if !s.inContent(rel) || path.Clean(rel) == s.ContentDir {
		return "", errors.New("that can't be deleted")
	}
	if s.TrashDir == "" {
		return "", errors.New("trash folder is not configured")
	}
	stamp := time.Now().Format("20060102-150405")
	trashed := map[string]string{}
	for _, filePath := range pagePaths(s.translationIndex(), rel) {
		unitPath := unit(filePath)
		if _, done := trashed[unitPath]; done {
			continue
		}
		src, err := s.abs(unitPath)
		if err != nil {
			return "", err
		}
		dest := filepath.Join(s.TrashDir, stamp, filepath.FromSlash(unitPath))
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
		trashed[unitPath] = dest
	}
	return trashed[unit(rel)], nil
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
		defer func() { _ = f.Close() }()
		return fsutil.CreateExclusive(target, f)
	})
}

// Reorder sets weights so pages appear in the given order (10, 20, 30... to
// leave room for manual tweaks). Every translation of a page gets the same
// weight. Only files whose weight changes are written.
func (s *Site) Reorder(paths []string) error {
	index := s.translationIndex()
	for i, rel := range paths {
		if st, err := os.Stat(filepath.Join(s.Root, filepath.FromSlash(rel))); err == nil && st.IsDir() {
			idx := s.sectionIndex(rel)
			if idx == "" {
				// a folder without settings gets an _index.md to hold its position
				doc, _ := frontmatter.Parse(nil)
				if err := doc.Set("title", humanize(path.Base(rel)), s.DefaultFormat()); err != nil {
					return err
				}
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
		want := (i + 1) * 10
		for _, filePath := range pagePaths(index, rel) {
			p, err := s.GetPage(filePath)
			if err != nil {
				return err
			}
			if p.ParseError != "" || toInt(p.Fields["weight"]) == want {
				continue
			}
			if _, err := s.SavePage(SaveRequest{Path: filePath, BaseHash: p.Hash, Set: map[string]any{"weight": want}}); err != nil {
				return err
			}
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
