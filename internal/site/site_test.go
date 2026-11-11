package site

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/myrheimb/pageroom/internal/fsutil"
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

func TestTreePairsTranslations(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"hugo.toml":                  "baseURL = 'https://example.org/'\ndefaultContentLanguage = 'en'\n[languages.en]\n  weight = 1\n[languages.ja]\n  weight = 2\n",
		"content/_index.md":          "+++\ntitle = 'Home'\n+++\n",
		"content/_index.ja.md":       "+++\ntitle = 'ホーム'\n+++\n",
		"content/about.md":           "+++\ntitle = 'About'\nweight = 1\n+++\n",
		"content/about.ja.md":        "+++\ntitle = '私たちについて'\nweight = 5\n+++\n",
		"content/only-ja.ja.md":      "+++\ntitle = '日本語だけ'\n+++\n",
		"content/only-en.md":         "+++\ntitle = 'English only'\n+++\n",
		"content/contact.md":         "+++\ntitle = 'Contact'\ntranslationKey = 'contact'\n+++\n",
		"content/otoiawase.ja.md":    "+++\ntitle = 'お問い合わせ'\ntranslationKey = 'contact'\n+++\n",
		"content/guide/index.md":     "+++\ntitle = 'Guide'\n+++\n",
		"content/guide/index.ja.md":  "+++\ntitle = 'ガイド'\n+++\n",
		"content/posts/_index.md":    "+++\ntitle = 'Blog'\n+++\n",
		"content/posts/_index.ja.md": "+++\ntitle = 'ブログ'\n+++\n",
		"content/posts/hello.md":     "+++\ntitle = 'Hello'\nurl = '/greeting/'\n+++\n",
		"content/posts/hello.ja.md":  "+++\ntitle = 'こんにちは'\nurl = '/ja/aisatsu/'\n+++\n",
		"content/posts/v1.2.md":      "+++\ntitle = 'Version'\n+++\n",
	}
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(c), 0o644)
	}
	s, err := Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}

	want := map[string][]Translation{
		"content/about.md":       {{"en", "content/about.md", "/about/"}, {"ja", "content/about.ja.md", "/ja/about/"}},
		"content/only-ja.ja.md":  {{"ja", "content/only-ja.ja.md", "/ja/only-ja/"}},
		"content/only-en.md":     {{"en", "content/only-en.md", "/only-en/"}},
		"content/contact.md":     {{"en", "content/contact.md", "/contact/"}, {"ja", "content/otoiawase.ja.md", "/ja/otoiawase/"}},
		"content/guide/index.md": {{"en", "content/guide/index.md", "/guide/"}, {"ja", "content/guide/index.ja.md", "/ja/guide/"}},
		"content/posts/hello.md": {{"en", "content/posts/hello.md", "/greeting/"}, {"ja", "content/posts/hello.ja.md", "/ja/aisatsu/"}},
		"content/posts/v1.2.md":  {{"en", "content/posts/v1.2.md", "/posts/v1.2/"}},
	}
	for p, translations := range want {
		node := find(tree, p)
		if node == nil {
			t.Errorf("%s missing from the tree", p)
			continue
		}
		if !slices.Equal(node.Translations, translations) {
			t.Errorf("%s translations: %+v", p, node.Translations)
		}
	}
	for _, gone := range []string{"content/about.ja.md", "content/otoiawase.ja.md", "content/guide/index.ja.md", "content/posts/hello.ja.md"} {
		if find(tree, gone) != nil {
			t.Errorf("%s should be folded into its pair", gone)
		}
	}
	if about := find(tree, "content/about.md"); about.Title != "About" || about.Weight != 1 {
		t.Errorf("about takes the default language: %+v", about)
	}
	if ja := find(tree, "content/only-ja.ja.md"); ja.Title != "日本語だけ" {
		t.Errorf("second-language-only page: %+v", ja)
	}
	posts := find(tree, "content/posts")
	if posts.IndexMD != "content/posts/_index.md" || posts.Title != "Blog" || len(posts.Translations) != 2 || posts.Translations[1].URL != "/ja/posts/" {
		t.Errorf("section: %+v", posts)
	}
	if len(tree.Children) != 6 {
		t.Errorf("want 6 top-level nodes, got %d", len(tree.Children))
	}
}

func languageFixture(t *testing.T) *Site {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"hugo.toml":                 "baseURL = 'https://example.org/'\ndefaultContentLanguage = 'en'\n[languages.en]\n  weight = 1\n[languages.ja]\n  weight = 2\n",
		"content/_index.md":         "+++\ntitle = 'Home'\n+++\n",
		"content/about.md":          "+++\ntitle = 'About'\n+++\n",
		"content/about.ja.md":       "+++\ntitle = '私たち'\nurl = '/ja/watashi/'\n+++\n",
		"content/solo.md":           "+++\ntitle = 'Solo'\n+++\n",
		"content/guide/index.md":    "+++\ntitle = 'Guide'\n+++\n",
		"content/guide/index.ja.md": "+++\ntitle = 'ガイド'\n+++\n",
		"content/guide/pic.jpg":     "jpg",
		"content/news/_index.md":    "+++\ntitle = 'News'\n+++\n",
		"content/contact.md":        "+++\ntitle = 'Contact'\ntranslationKey = 'contact'\n+++\n",
		"content/otoiawase.ja.md":   "+++\ntitle = 'お問い合わせ'\ntranslationKey = 'contact'\n+++\n",
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

func TestMoveActsOnEveryTranslation(t *testing.T) {
	s := languageFixture(t)
	got, err := s.MovePage("content/about.md", "content/news")
	if err != nil || got != "content/news/about.md" {
		t.Fatalf("%s %v", got, err)
	}
	if !fsutil.Exists(filepath.Join(s.Root, "content/news/about.ja.md")) || fsutil.Exists(filepath.Join(s.Root, "content/about.ja.md")) || fsutil.Exists(filepath.Join(s.Root, "content/about.md")) {
		t.Fatal("both files should have moved")
	}
	en, _ := s.GetPage("content/news/about.md")
	if a, _ := en.Fields["aliases"].([]any); len(a) != 1 || a[0] != "/about/" {
		t.Fatalf("en aliases = %v", en.Fields["aliases"])
	}

	if _, err := s.MovePage("content/solo.md", "content/news"); err != nil || !fsutil.Exists(filepath.Join(s.Root, "content/news/solo.md")) {
		t.Fatalf("one-language page: %v", err)
	}

	got, err = s.MovePage("content/guide/index.ja.md", "content/news")
	if err != nil || got != "content/news/guide/index.ja.md" {
		t.Fatalf("%s %v", got, err)
	}
	for _, p := range []string{"content/news/guide/index.md", "content/news/guide/index.ja.md", "content/news/guide/pic.jpg"} {
		if !fsutil.Exists(filepath.Join(s.Root, p)) {
			t.Errorf("%s missing", p)
		}
	}
	ja, _ := s.GetPage("content/news/guide/index.ja.md")
	if a, _ := ja.Fields["aliases"].([]any); len(a) != 1 || a[0] != "/ja/guide/" {
		t.Fatalf("bundle ja aliases = %v", ja.Fields["aliases"])
	}
}

func TestDeleteActsOnEveryTranslation(t *testing.T) {
	s := languageFixture(t)
	if _, err := s.DeletePage("content/about.md"); err != nil {
		t.Fatal(err)
	}
	if fsutil.Exists(filepath.Join(s.Root, "content/about.md")) || fsutil.Exists(filepath.Join(s.Root, "content/about.ja.md")) {
		t.Fatal("a translation was left behind")
	}
	trashed, _ := filepath.Glob(filepath.Join(s.TrashDir, "*", "content", "about*.md"))
	if len(trashed) != 2 {
		t.Fatalf("trash holds %v", trashed)
	}

	if _, err := s.DeletePage("content/solo.md"); err != nil || fsutil.Exists(filepath.Join(s.Root, "content/solo.md")) {
		t.Fatal(err)
	}
	if _, err := s.DeletePage("content/guide/index.md"); err != nil || fsutil.Exists(filepath.Join(s.Root, "content/guide")) {
		t.Fatal("bundle with both languages should go as one")
	}
}

func TestReorderWritesEveryTranslation(t *testing.T) {
	s := languageFixture(t)
	if err := s.Reorder([]string{"content/solo.md", "content/about.md"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"content/about.md", "content/about.ja.md"} {
		page, _ := s.GetPage(p)
		if toInt(page.Fields["weight"]) != 20 {
			t.Errorf("%s weight %v", p, page.Fields["weight"])
		}
	}
	solo, _ := s.GetPage("content/solo.md")
	if toInt(solo.Fields["weight"]) != 10 {
		t.Errorf("solo weight %v", solo.Fields["weight"])
	}
}

func TestRenameKeepsLanguageCodeAndAliasesUrl(t *testing.T) {
	s := languageFixture(t)
	got, err := s.RenameSlug("content/about.md", "Our team")
	if err != nil || got != "content/our-team.md" {
		t.Fatalf("%s %v", got, err)
	}
	if fsutil.Exists(filepath.Join(s.Root, "content/about.md")) || fsutil.Exists(filepath.Join(s.Root, "content/about.ja.md")) || !fsutil.Exists(filepath.Join(s.Root, "content/our-team.ja.md")) {
		t.Fatal("both files should carry the new name")
	}
	ja, _ := s.GetPage("content/our-team.ja.md")
	if _, hasURL := ja.Fields["url"]; hasURL {
		t.Error("url should be dropped")
	}
	if ja.URL != "/ja/our-team/" {
		t.Errorf("url %s", ja.URL)
	}
	if a, _ := ja.Fields["aliases"].([]any); len(a) != 1 || a[0] != "/ja/watashi/" {
		t.Fatalf("aliases = %v", ja.Fields["aliases"])
	}

	got, err = s.RenameSlug("content/otoiawase.ja.md", "Reach us")
	if err != nil || got != "content/reach-us.ja.md" || !fsutil.Exists(filepath.Join(s.Root, "content/reach-us.md")) {
		t.Fatalf("translationKey pair: %s %v", got, err)
	}

	got, err = s.RenameSlug("content/guide/index.ja.md", "Handbook")
	if err != nil || got != "content/handbook/index.ja.md" || !fsutil.Exists(filepath.Join(s.Root, "content/handbook/index.md")) {
		t.Fatalf("bundle: %s %v", got, err)
	}

	if got, err = s.RenameSlug("content/solo.md", "Solo"); err != nil || got != "content/solo.md" {
		t.Fatalf("same name: %s %v", got, err)
	}
	if got, err = s.RenameSlug("content/solo.md", "Alone"); err != nil || got != "content/alone.md" {
		t.Fatalf("one language: %s %v", got, err)
	}
}

func TestCreatePageInLanguage(t *testing.T) {
	s := languageFixture(t)
	got, err := s.CreatePage(NewPage{Section: "content", Title: "Fresh", Language: "ja"})
	if err != nil || got != "content/fresh.ja.md" {
		t.Fatalf("%s %v", got, err)
	}
	again, _ := s.CreatePage(NewPage{Section: "content", Title: "Fresh", Language: "ja"})
	if again != "content/fresh-2.ja.md" {
		t.Fatalf("second: %s", again)
	}
	bundle, _ := s.CreatePage(NewPage{Section: "content", Title: "Pack", Bundle: true, Language: "ja"})
	if bundle != "content/pack/index.ja.md" {
		t.Fatalf("bundle: %s", bundle)
	}
	plain, _ := s.CreatePage(NewPage{Section: "content", Title: "Plain", Language: "en"})
	if plain != "content/plain.md" {
		t.Fatalf("default language: %s", plain)
	}
	if _, err := s.CreatePage(NewPage{Section: "content", Title: "X", Language: "fr"}); err == nil {
		t.Fatal("created a page in a language the site lacks")
	}
}

func TestAddTranslationCreatesPairedFile(t *testing.T) {
	s := languageFixture(t)
	got, err := s.AddTranslation("content/solo.md", "ja")
	if err != nil || got != "content/solo.ja.md" {
		t.Fatalf("%s %v", got, err)
	}
	tree, _ := s.Tree()
	node := find(tree, "content/solo.md")
	if node == nil || len(node.Translations) != 2 || find(tree, "content/solo.ja.md") != nil {
		t.Fatalf("tree should group the pair: %+v", node)
	}
	if page, _ := s.GetPage(got); page.Fields["title"] != "Solo" || page.Fields["draft"] != true {
		t.Fatalf("fields %v", page.Fields)
	}
	if _, err := s.AddTranslation("content/solo.md", "ja"); err == nil {
		t.Fatal("added a language the page has")
	}
	if _, err := s.AddTranslation("content/solo.md", "fr"); err == nil {
		t.Fatal("added an unknown language")
	}

	got, err = s.AddTranslation("content/guide/index.md", "ja")
	if err == nil {
		t.Fatalf("guide already has ja, got %s", got)
	}
	sec, err := s.AddTranslation("content/news", "ja")
	if err != nil || sec != "content/news/_index.ja.md" {
		t.Fatalf("section: %s %v", sec, err)
	}
}

func TestRenameClashKeepsPairTogether(t *testing.T) {
	s := languageFixture(t)
	taken := filepath.Join(s.Root, "content/our-team.ja.md")
	if err := os.WriteFile(taken, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.RenameSlug("content/about.md", "Our team")
	if err != nil || got != "content/our-team-2.md" {
		t.Fatalf("%s %v", got, err)
	}

	content, _ := os.ReadFile(taken)
	if string(content) != "keep me" || !fsutil.Exists(filepath.Join(s.Root, "content/our-team-2.ja.md")) {
		t.Fatal("the taken file was overwritten or the pair split")
	}
}

func TestMoveClashKeepsPairTogether(t *testing.T) {
	s := languageFixture(t)
	if err := os.MkdirAll(filepath.Join(s.Root, "content/news"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "content/news/about.md"), []byte("+++\ntitle = 'Other'\n+++\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.MovePage("content/about.md", "content/news")
	if err != nil || got != "content/news/about-2.md" {
		t.Fatalf("%s %v", got, err)
	}
	if !fsutil.Exists(filepath.Join(s.Root, "content/news/about-2.ja.md")) || fsutil.Exists(filepath.Join(s.Root, "content/news/about.ja.md")) {
		t.Fatal("the pair should share one stem")
	}
}

func TestSaveMenuKeepsKeysItDoesNotEdit(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"hugo.toml":         "baseURL = 'https://example.org/'\n",
		"content/_index.md": "+++\ntitle = 'Home'\n+++\n",
		"config/_default/menus.toml": "[[main]]\n  identifier = 'docs'\n  name = 'Docs'\n  pageRef = '/docs'\n  weight = 10\n  unknownKey = 'keep'\n  [main.params]\n    icon = 'book'\n\n" +
			"[[main]]\n  identifier = 'blog'\n  name = 'Blog'\n  pageRef = '/blog'\n  weight = 20\n  [main.params]\n    icon = 'pen'\n",
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
	menus, _ := s.Menus()
	items := menus[0].Items
	items = []MenuItem{items[1], items[0]}
	items[1].Name = "Documentation"
	items = append(items, MenuItem{Name: "New", URL: "https://example.org"})
	if err := s.SaveMenu(Menu{Name: "main", Items: items}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "config/_default/menus.toml"))
	got := string(b)
	for _, want := range []string{"Documentation", "unknownKey", "'keep'", "icon", "'book'", "'pen'"} {
		if !strings.Contains(got, want) && !strings.Contains(got, strings.ReplaceAll(want, "'", `"`)) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
	s.Reload()
	menus, _ = s.Menus()
	if n := menus[0].Items; n[0].Name != "Blog" || n[1].Name != "Documentation" || n[2].Name != "New" {
		t.Fatalf("order: %+v", n)
	}
	if strings.Count(got, "params") != 2 || strings.Count(got, "unknownKey") != 1 {
		t.Fatalf("new entry got extra keys or old ones doubled:\n%s", got)
	}
}

func openFiles(t *testing.T, files map[string]string) *Site {
	t.Helper()
	root := t.TempDir()
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

func menuIn(menus []Menu, language, name string) Menu {
	for _, m := range menus {
		if m.Language == language && m.Name == name {
			return m
		}
	}
	return Menu{}
}

func TestMenusPerLanguage(t *testing.T) {
	enMenus := "[[main]]\n  identifier = 'docs'\n  name = 'Docs'\n  pageRef = '/docs'\n  weight = 10\n  [main.params]\n    icon = 'book'\n"
	jaMenus := "[[main]]\n  identifier = 'docs'\n  name = 'ドキュメント'\n  pageRef = '/docs'\n  weight = 10\n  [main.params]\n    icon = 'book'\n"
	s := openFiles(t, map[string]string{
		"hugo.toml":                     "baseURL = 'https://example.org/'\ndefaultContentLanguage = 'en'\n[languages.en]\n  weight = 1\n[languages.ja]\n  weight = 2\n",
		"content/_index.md":             "+++\ntitle = 'Home'\n+++\n",
		"config/_default/menus.en.toml": enMenus,
		"config/_default/menus.ja.toml": jaMenus,
	})
	menus, err := s.Menus()
	if err != nil {
		t.Fatal(err)
	}
	if got := menuIn(menus, "en", "main").Items; len(got) != 1 || got[0].Name != "Docs" {
		t.Fatalf("en: %+v", menus)
	}
	if got := menuIn(menus, "ja", "main").Items; len(got) != 1 || got[0].Name != "ドキュメント" {
		t.Fatalf("ja: %+v", menus)
	}

	items := menuIn(menus, "ja", "main").Items
	items[0].Name = "資料"
	if err := s.SaveMenu(Menu{Name: "main", Language: "ja", Items: items}); err != nil {
		t.Fatal(err)
	}
	en, _ := os.ReadFile(filepath.Join(s.Root, "config/_default/menus.en.toml"))
	if string(en) != enMenus {
		t.Fatalf("en file changed:\n%s", en)
	}
	ja, _ := os.ReadFile(filepath.Join(s.Root, "config/_default/menus.ja.toml"))
	if !strings.Contains(string(ja), "資料") || !strings.Contains(string(ja), "icon") {
		t.Fatalf("ja file:\n%s", ja)
	}

	items = menuIn(menus, "en", "main").Items
	items[0].Name = "Documentation"
	if err := s.SaveMenu(Menu{Name: "main", Language: "en", Items: items}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, "config/_default/menus.ja.toml"))
	if string(after) != string(ja) {
		t.Fatalf("ja file changed by an en save:\n%s", after)
	}
}

func TestMenusFallBackToSharedConfig(t *testing.T) {
	s := openFiles(t, map[string]string{
		"hugo.toml":                  "baseURL = 'https://example.org/'\n[languages.en]\n  weight = 1\n[languages.ja]\n  weight = 2\n",
		"content/_index.md":          "+++\ntitle = 'Home'\n+++\n",
		"config/_default/menus.toml": "[[main]]\n  identifier = 'docs'\n  name = 'Docs'\n  pageRef = '/docs'\n  weight = 10\n  [main.params]\n    icon = 'book'\n",
	})
	menus, _ := s.Menus()
	items := menuIn(menus, "ja", "main").Items
	if len(items) != 1 || items[0].Name != "Docs" {
		t.Fatalf("ja should fall back to menus.toml: %+v", menus)
	}
	items[0].Name = "資料"
	if err := s.SaveMenu(Menu{Name: "main", Language: "ja", Items: items}); err != nil {
		t.Fatal(err)
	}
	shared, _ := os.ReadFile(filepath.Join(s.Root, "config/_default/menus.toml"))
	if !strings.Contains(string(shared), "Docs") || strings.Contains(string(shared), "資料") {
		t.Fatalf("shared file changed:\n%s", shared)
	}
	ja, _ := os.ReadFile(filepath.Join(s.Root, "config/_default/menus.ja.toml"))
	if !strings.Contains(string(ja), "資料") || !strings.Contains(string(ja), "icon") {
		t.Fatalf("ja file:\n%s", ja)
	}
	s.Reload()
	menus, _ = s.Menus()
	if menuIn(menus, "en", "main").Items[0].Name != "Docs" || menuIn(menus, "ja", "main").Items[0].Name != "資料" {
		t.Fatalf("after save: %+v", menus)
	}
}

func TestSingleLanguageMenusCarryTheDefaultLanguage(t *testing.T) {
	s := openFiles(t, map[string]string{
		"hugo.toml":                  "baseURL = 'https://example.org/'\n",
		"content/_index.md":          "+++\ntitle = 'Home'\n+++\n",
		"config/_default/menus.toml": "[[main]]\n  name = 'Docs'\n  pageRef = '/docs'\n",
	})
	menus, _ := s.Menus()
	if len(menus) != 1 || menus[0].Language != "en" || menus[0].Items[0].Name != "Docs" {
		t.Fatalf("menus = %+v", menus)
	}
}

func TestLanguagesFileGivesTheSameTreeAsMainConfig(t *testing.T) {
	content := map[string]string{
		"content/_index.md":         "+++\ntitle = 'Home'\n+++\n",
		"content/about.md":          "+++\ntitle = 'About'\n+++\n",
		"content/about.ja.md":       "+++\ntitle = '私たちについて'\n+++\n",
		"content/only-ja.ja.md":     "+++\ntitle = '日本語だけ'\n+++\n",
		"content/guide/index.md":    "+++\ntitle = 'Guide'\n+++\n",
		"content/guide/index.ja.md": "+++\ntitle = 'ガイド'\n+++\n",
	}
	inMain := map[string]string{"hugo.toml": "baseURL = 'https://example.org/'\ndefaultContentLanguage = 'en'\n[languages.en]\n  weight = 1\n[languages.ja]\n  weight = 2\n"}
	inFile := map[string]string{
		"hugo.toml":                      "baseURL = 'https://example.org/'\ndefaultContentLanguage = 'en'\n",
		"config/_default/languages.toml": "[en]\n  weight = 1\n[ja]\n  weight = 2\n",
	}
	trees := make([]*Node, 2)
	for index, extra := range []map[string]string{inMain, inFile} {
		files := maps.Clone(content)
		maps.Copy(files, extra)
		tree, err := openFiles(t, files).Tree()
		if err != nil {
			t.Fatal(err)
		}
		trees[index] = tree
	}
	if !reflect.DeepEqual(trees[0], trees[1]) {
		t.Fatalf("trees differ:\n%+v\n%+v", trees[0], trees[1])
	}
	about := find(trees[1], "content/about.md")
	if about == nil || len(about.Translations) != 2 || about.Translations[1].URL != "/ja/about/" {
		t.Fatalf("about = %+v", about)
	}
}
