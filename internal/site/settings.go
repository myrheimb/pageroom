package site

import (
	"cmp"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/myrheimb/pageroom/internal/fsutil"
)

type Settings struct {
	Title       string `json:"title"`
	BaseURL     string `json:"baseURL"`
	Language    string `json:"language"`
	Description string `json:"description"`
	Copyright   string `json:"copyright"`
	Theme       string `json:"theme"`
	// FormAddress is where contact forms send messages: a form service's
	// web address. Without it, they show ContactEmail instead.
	FormAddress  string `json:"formAddress"`
	ContactEmail string `json:"contactEmail"`
}

func (s *Site) Settings() Settings {
	c := s.Config
	params := c.Map("params")
	desc, _ := params["description"].(string)
	form, _ := params["contactForm"].(map[string]any)
	action, _ := form["action"].(string)
	email, _ := form["email"].(string)
	return Settings{
		Title: c.String("title"), BaseURL: c.String("baseURL"), Language: s.languageValue(),
		Description: desc, Copyright: c.String("copyright"), Theme: c.Theme(),
		FormAddress: action, ContactEmail: email,
	}
}

// SaveSettings writes only the values that changed, keeping the config
// file's comments and layout.
func (s *Site) SaveSettings(n Settings) error {
	old := s.Settings()
	set := func(key, was, now string) error {
		if was == now {
			return nil
		}
		if now == "" {
			s.Config.doc.Delete(key)
			return nil
		}
		return s.Config.doc.Set(key, now, s.Config.Format())
	}
	if err := set("title", old.Title, n.Title); err != nil {
		return err
	}
	if err := set("baseURL", old.BaseURL, n.BaseURL); err != nil {
		return err
	}
	if err := set(s.languageKey(), old.Language, n.Language); err != nil {
		return err
	}
	if err := set("copyright", old.Copyright, n.Copyright); err != nil {
		return err
	}
	if err := set("theme", old.Theme, n.Theme); err != nil {
		return err
	}
	formChanged := old.FormAddress != n.FormAddress || old.ContactEmail != n.ContactEmail
	if old.Description != n.Description || formChanged {
		params := s.Config.Map("params")
		if params == nil {
			params = map[string]any{}
		}
		if old.Description != n.Description {
			params["description"] = n.Description
		}
		if formChanged {
			form, _ := params["contactForm"].(map[string]any)
			if form == nil {
				form = map[string]any{}
			}
			setOrDelete(form, "action", n.FormAddress)
			setOrDelete(form, "email", n.ContactEmail)
			if len(form) == 0 {
				delete(params, "contactForm")
			} else {
				params["contactForm"] = form
			}
		}
		if err := s.Config.doc.Set("params", params, s.Config.Format()); err != nil {
			return err
		}
	}
	return s.saveConfig()
}

// Logo is the image path in params.logo, which Pageroom's starter themes
// show in the header.
func (s *Site) Logo() string {
	logo, _ := s.Config.Map("params")["logo"].(string)
	return logo
}

func (s *Site) SetLogo(path string) error {
	params := s.Config.Map("params")
	if params == nil {
		params = map[string]any{}
	}
	params["logo"] = path
	if err := s.Config.doc.Set("params", params, s.Config.Format()); err != nil {
		return err
	}
	return s.saveConfig()
}

func setOrDelete(m map[string]any, key, value string) {
	if value == "" {
		delete(m, key)
	} else {
		m[key] = value
	}
}

// Hugo 0.158 renamed languageCode to locale. Use whichever the site has.
func (s *Site) languageKey() string {
	if _, ok := s.Config.doc.Get("languageCode"); ok {
		return "languageCode"
	}
	return "locale"
}

func (s *Site) languageValue() string {
	if v := s.Config.String("locale"); v != "" {
		return v
	}
	return s.Config.String("languageCode")
}

func (s *Site) saveConfig() error {
	out, err := s.Config.doc.Bytes()
	if err != nil {
		return err
	}
	full, err := s.abs(s.Config.Path)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(full, out, 0o644)
}

func (s *Site) Reload() error {
	cfg, err := loadConfig(s.Root, s.Config.Path)
	if err != nil {
		return err
	}
	s.Config = cfg
	if p, err := loadPrefs(s.Root); err == nil {
		s.Prefs = p
	}
	return nil
}

func (s *Site) Themes() []string {
	entries, err := os.ReadDir(filepath.Join(s.Root, "themes"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

func (s *Site) defaultLanguage() string {
	if v := s.Config.String("defaultContentLanguage"); v != "" {
		return v
	}
	return "en"
}

func (s *Site) defaultInSubdir() bool {
	v, _ := s.Config.doc.Get("defaultContentLanguageInSubdir")
	b, _ := v.(bool)
	return b
}

// contentLanguages are the site's language codes, the default first and
// the rest by weight, then by code.
func (s *Site) contentLanguages() []string {
	def := s.defaultLanguage()
	configured, _ := s.Config.doc.Get("languages")
	table, _ := configured.(map[string]any)
	others := make([]string, 0, len(table))
	for code := range table {
		if code != def {
			others = append(others, code)
		}
	}
	weightOf := func(code string) int {
		entry, _ := table[code].(map[string]any)
		if w := toInt(entry["weight"]); w > 0 {
			return w
		}
		return int(^uint(0) >> 1)
	}
	slices.SortFunc(others, func(a, b string) int {
		return cmp.Or(cmp.Compare(weightOf(a), weightOf(b)), cmp.Compare(a, b))
	})
	return append([]string{def}, others...)
}

// splitLanguage takes the language code out of a content file name, as in
// "about.ja.md", and falls back to the default language.
func (s *Site) splitLanguage(name string) (stem, lang string) {
	stem = strings.TrimSuffix(name, path.Ext(name))
	if dot := strings.LastIndex(stem, "."); dot >= 0 {
		if code := stem[dot+1:]; slices.Contains(s.contentLanguages(), code) {
			return stem[:dot], code
		}
	}
	return stem, s.defaultLanguage()
}
