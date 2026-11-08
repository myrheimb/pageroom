package site

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) *Site {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"hugo.toml":                     "# config\nbaseURL = 'https://example.org/'\ntitle = 'My Site'\n\n[[menus.main]]\n  name = 'Blog'\n  pageRef = '/posts'\n  weight = 20\n",
		"content/_index.md":             "+++\ntitle = 'Home'\n+++\n",
		"content/about.md":              "+++\ntitle = 'About us'\nweight = 1\nmenus = 'main'\n+++\nHello\n",
		"content/posts/_index.md":       "+++\ntitle = 'Blog'\n+++\n",
		"content/posts/hello/index.md":  "---\ntitle: Hello world\ndate: 2026-01-02T10:00:00Z\n---\nBody\n",
		"content/posts/hello/beach.jpg": "jpg",
		"content/posts/second.md":       "---\ntitle: Second\ndate: 2026-02-01T10:00:00Z\ndraft: true\n---\n",
		"content/posts/broken.md":       "---\ntitle: [oops\n---\n",
		"archetypes/default.md":         "+++\ntitle = '{{ .Name }}'\n+++\n",
	}
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(c), 0o644)
	}
	s, err := Open(root, filepath.Join(t.TempDir(), "trash"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func find(n *Node, p string) *Node {
	if n.Path == p {
		return n
	}
	for _, c := range n.Children {
		if f := find(c, p); f != nil {
			return f
		}
	}
	return nil
}

func TestTree(t *testing.T) {
	s := fixture(t)
	tree, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}
	posts := find(tree, "content/posts")
	if posts == nil || posts.Kind != "section" || posts.Title != "Blog" || posts.IndexMD != "content/posts/_index.md" {
		t.Fatalf("posts section: %+v", posts)
	}
	hello := find(tree, "content/posts/hello/index.md")
	if hello == nil || !hello.Bundle || hello.URL != "/posts/hello/" {
		t.Fatalf("bundle: %+v", hello)
	}
	// newest first within equal weights
	if posts.Children[0].Title != "Second" {
		t.Errorf("order: %s first", posts.Children[0].Title)
	}
	if b := find(tree, "content/posts/broken.md"); b == nil || b.Error == "" {
		t.Errorf("broken page should be listed with an error: %+v", b)
	}
}

func TestSaveDetectsChangesOnDisk(t *testing.T) {
	s := fixture(t)
	p, _ := s.GetPage("content/about.md")
	os.WriteFile(filepath.Join(s.Root, "content/about.md"), []byte("+++\ntitle = 'Changed by sync'\n+++\n"), 0o644)
	body := "New body"
	_, err := s.SavePage(SaveRequest{Path: p.Path, BaseHash: p.Hash, Body: &body})
	var ce *ChangedOnDiskError
	if !errors.As(err, &ce) {
		t.Fatalf("want ChangedOnDiskError, got %v", err)
	}
}

func TestSaveKeepsFormatAndTypes(t *testing.T) {
	s := fixture(t)
	p, _ := s.GetPage("content/posts/hello/index.md")
	body := "Updated **body**"
	np, err := s.SavePage(SaveRequest{Path: p.Path, BaseHash: p.Hash, Body: &body,
		Set: map[string]any{"date": "2026-03-04T05:06:07Z", "weight": float64(5), "tags": []any{"a"}}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.Root, p.Path))
	got := string(b)
	for _, want := range []string{"date: 2026-03-04T05:06:07Z", "weight: 5\n", "Updated **body**\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if np.Hash == p.Hash {
		t.Error("hash not updated")
	}
}

func TestCreatePageNeverOverwrites(t *testing.T) {
	s := fixture(t)
	a, err := s.CreatePage(NewPage{Section: "content/posts", Title: "Hello world", Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	if a != "content/posts/hello-world/index.md" {
		t.Fatalf("path %s", a)
	}
	b, _ := s.CreatePage(NewPage{Section: "content/posts", Title: "Hello world", Bundle: true})
	if b == a {
		t.Fatal("overwrote existing page")
	}
	raw, _ := os.ReadFile(filepath.Join(s.Root, a))
	if !strings.HasPrefix(string(raw), "+++\n") || !strings.Contains(string(raw), "draft = true") {
		t.Fatalf("new page should follow the archetype's TOML: %s", raw)
	}
	if _, err := s.CreatePage(NewPage{Section: "../outside", Title: "x"}); err == nil {
		t.Fatal("created outside content")
	}
}

func TestMoveAddsAliasAndCarriesImages(t *testing.T) {
	s := fixture(t)
	sec, _ := s.CreateSection("content", "News")
	newPath, err := s.MovePage("content/posts/hello/index.md", sec)
	if err != nil {
		t.Fatal(err)
	}
	if newPath != "content/news/hello/index.md" {
		t.Fatalf("moved to %s", newPath)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "content/news/hello/beach.jpg")); err != nil {
		t.Fatal("bundle image did not move")
	}
	p, _ := s.GetPage(newPath)
	if a, _ := p.Fields["aliases"].([]any); len(a) != 1 || a[0] != "/posts/hello/" {
		t.Fatalf("aliases = %v", p.Fields["aliases"])
	}
}

func TestRenameSlug(t *testing.T) {
	s := fixture(t)
	np, err := s.RenameSlug("content/about.md", "About the team")
	if err != nil || np != "content/about-the-team.md" {
		t.Fatalf("%s %v", np, err)
	}
	p, _ := s.GetPage(np)
	if p.URL != "/about-the-team/" {
		t.Fatalf("url %s", p.URL)
	}
}

func TestDeleteGoesToTrash(t *testing.T) {
	s := fixture(t)
	dest, err := s.DeletePage("content/posts/hello/index.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "beach.jpg")); err != nil {
		t.Fatal("bundle not in trash")
	}
	if _, err := s.DeletePage("content"); err == nil {
		t.Fatal("deleted the whole content folder")
	}
}

func TestReorder(t *testing.T) {
	s := fixture(t)
	if err := s.Reorder([]string{"content/posts/second.md", "content/posts/hello/index.md"}); err != nil {
		t.Fatal(err)
	}
	p, _ := s.GetPage("content/posts/hello/index.md")
	if toInt(p.Fields["weight"]) != 20 {
		t.Fatalf("weight %v", p.Fields["weight"])
	}
}

func TestMenusMergeAndSave(t *testing.T) {
	s := fixture(t)
	menus, err := s.Menus()
	if err != nil {
		t.Fatal(err)
	}
	if len(menus) != 1 || menus[0].Name != "main" || len(menus[0].Items) != 2 {
		t.Fatalf("menus = %+v", menus)
	}
	// About (front matter, weight 0) sorts before Blog (config, weight 20)
	items := menus[0].Items
	if items[0].Name != "About us" || items[0].Source != "content/about.md" {
		t.Fatalf("items = %+v", items)
	}
	items = append([]MenuItem{items[1], items[0]}, MenuItem{Name: "GitHub", URL: "https://github.com/x"})
	if err := s.SaveMenu(Menu{Name: "main", Items: items}); err != nil {
		t.Fatal(err)
	}
	s.Reload()
	menus, _ = s.Menus()
	got := menus[0].Items
	if len(got) != 3 || got[0].Name != "Blog" || got[1].Name != "About us" || got[1].Source != "config" || got[2].URL == "" {
		t.Fatalf("after save: %+v", got)
	}
	about, _ := s.GetPage("content/about.md")
	if _, ok := about.Fields["menus"]; ok {
		t.Fatal("front matter menu entry should be moved into config")
	}
	cfg, _ := os.ReadFile(filepath.Join(s.Root, "hugo.toml"))
	if !strings.HasPrefix(string(cfg), "# config\n") {
		t.Fatalf("config comment lost:\n%s", cfg)
	}
}

func TestSettings(t *testing.T) {
	s := fixture(t)
	st := s.Settings()
	st.Title = "Renamed"
	st.Description = "A lovely site"
	if err := s.SaveSettings(st); err != nil {
		t.Fatal(err)
	}
	s.Reload()
	if got := s.Settings(); got.Title != "Renamed" || got.Description != "A lovely site" {
		t.Fatalf("%+v", got)
	}

	st = s.Settings()
	st.FormAddress = "https://formspree.io/f/abc"
	st.ContactEmail = "hello@example.org"
	if err := s.SaveSettings(st); err != nil {
		t.Fatal(err)
	}
	s.Reload()
	if got := s.Settings(); got != st {
		t.Fatalf("contact form not saved: %+v", got)
	}

	st.FormAddress, st.ContactEmail = "", ""
	if err := s.SaveSettings(st); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(filepath.Join(s.Root, "hugo.toml"))
	if strings.Contains(string(cfg), "contactForm") || !strings.Contains(string(cfg), "A lovely site") {
		t.Fatalf("clearing the contact form should remove only its table:\n%s", cfg)
	}
}

func TestAddContactForm(t *testing.T) {
	for language, want := range map[string]string{"": "Send", "ja-JP": "送信"} {
		s := fixture(t)
		if language != "" {
			st := s.Settings()
			st.Language = language
			if err := s.SaveSettings(st); err != nil {
				t.Fatal(err)
			}
			s.Reload()
		}
		if err := s.AddContactForm(); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(s.Root, contactFormFile)
		got, _ := os.ReadFile(file)
		if !strings.Contains(string(got), ">"+want+"<") || strings.Contains(string(got), "[[") {
			t.Errorf("%q: labels not filled in:\n%s", language, got)
		}

		os.WriteFile(file, []byte("the developer's own"), 0o644)
		if err := s.AddContactForm(); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(file); string(got) != "the developer's own" {
			t.Error("replaced a contact form the site already had")
		}
	}
}
