package site

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PrefsFile is the optional per-site file a developer can commit to tailor
// Pageroom for the people who edit the site.
const PrefsFile = ".pageroom.yaml"

type Prefs struct {
	Version    int                     `yaml:"version" json:"version"`
	Hugo       HugoPrefs               `yaml:"hugo" json:"hugo"`
	Sections   map[string]SectionPrefs `yaml:"sections" json:"sections"`
	Fields     []FieldDef              `yaml:"fields" json:"fields"`
	Menus      []string                `yaml:"menus" json:"menus"`
	Media      MediaPrefs              `yaml:"media" json:"media"`
	Appearance AppearancePrefs         `yaml:"appearance" json:"appearance"`
}

type HugoPrefs struct {
	Version string `yaml:"version" json:"version"`
}

type MediaPrefs struct {
	MaxWidth int    `yaml:"maxWidth" json:"maxWidth"`
	Folder   string `yaml:"folder" json:"folder"` // for pages that are not bundles, default static/images
}

type AppearancePrefs struct {
	HeadHook string `yaml:"headHook" json:"headHook"`
}

type SectionPrefs struct {
	Label  string `yaml:"label" json:"label"`
	Icon   string `yaml:"icon" json:"icon"`
	Hidden bool   `yaml:"hidden" json:"hidden"`
}

type FieldDef struct {
	Key      string     `yaml:"key" json:"key"`
	Label    string     `yaml:"label" json:"label"`
	Type     string     `yaml:"type" json:"type"` // text, textarea, number, boolean, date, image, list, select, repeat
	Options  []string   `yaml:"options" json:"options,omitempty"`
	Help     string     `yaml:"help" json:"help,omitempty"`
	Sections []string   `yaml:"sections" json:"sections,omitempty"` // limit to these sections
	Pages    []string   `yaml:"pages" json:"pages,omitempty"`       // limit to these pages and their language twins, such as content/solution.md
	Fields   []FieldDef `yaml:"fields" json:"fields,omitempty"`     // the keys of each item of a repeat field
}

func loadPrefs(root string) (*Prefs, error) {
	p := &Prefs{}
	b, err := os.ReadFile(filepath.Join(root, PrefsFile))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, p); err != nil {
		return nil, err
	}
	return p, nil
}
