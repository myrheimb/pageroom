package site

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/myrheimb/pageroom/internal/frontmatter"
	"github.com/myrheimb/pageroom/internal/fsutil"
)

// configCandidates in Hugo's lookup order.
var configCandidates = []string{
	"hugo.toml", "hugo.yaml", "hugo.yml", "hugo.json",
	"config.toml", "config.yaml", "config.yml", "config.json",
	"config/_default/hugo.toml", "config/_default/hugo.yaml", "config/_default/hugo.yml", "config/_default/hugo.json",
	"config/_default/config.toml", "config/_default/config.yaml", "config/_default/config.yml", "config/_default/config.json",
}

var ErrNotHugo = errors.New("this folder doesn't look like a Hugo site (no hugo.toml or config file found)")

func findConfig(root string) (string, error) {
	for _, c := range configCandidates {
		if fsutil.Exists(filepath.Join(root, filepath.FromSlash(c))) {
			return c, nil
		}
	}
	return "", ErrNotHugo
}

type Config struct {
	Path string // relative to site root
	doc  *frontmatter.Document
}

func loadConfig(root, rel string) (*Config, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	doc, err := frontmatter.ParseData(b, frontmatter.FormatFromExt(filepath.Ext(rel)))
	if err != nil {
		return nil, err
	}
	return &Config{Path: rel, doc: doc}, nil
}

func (c *Config) String(key string) string {
	v, _ := c.doc.Get(key)
	s, _ := v.(string)
	return s
}

func (c *Config) Map(key string) map[string]any {
	v, _ := c.doc.Get(key)
	m, _ := v.(map[string]any)
	return m
}

// Format is the syntax of the config file. New pages default to it so a
// TOML site keeps TOML front matter.
func (c *Config) Format() frontmatter.Format { return c.doc.Format }

func (c *Config) Theme() string {
	v, _ := c.doc.Get("theme")
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if len(t) > 0 {
			s, _ := t[0].(string)
			return s
		}
	}
	return ""
}
