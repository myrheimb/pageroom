package wordpress

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/myrheimb/pageroom/internal/appearance"
	"github.com/myrheimb/pageroom/internal/favicon"
	"github.com/myrheimb/pageroom/internal/frontmatter"
	"github.com/myrheimb/pageroom/internal/fsutil"
	"github.com/myrheimb/pageroom/internal/media"
	"github.com/myrheimb/pageroom/internal/site"
)

type Fetcher interface {
	Get(ctx context.Context, rawURL string) ([]byte, error)
}

// HTTPFetcher downloads over HTTP(S) with a size cap. It refuses private
// network addresses, so a crafted export can't make Pageroom probe the local
// network.
type HTTPFetcher struct{ Client *http.Client }

var errPrivate = errors.New("refusing to download from a private network address")

func NewHTTPFetcher() *HTTPFetcher {
	dialer := &net.Dialer{
		Timeout: 15 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
				return errPrivate
			}
			return nil
		},
	}
	tr := &http.Transport{DialContext: dialer.DialContext, Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 15 * time.Second}
	return &HTTPFetcher{Client: &http.Client{Transport: tr, Timeout: 2 * time.Minute}}
}

func (f *HTTPFetcher) Get(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("not a web address: %s", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Pageroom WordPress importer")
	// Google Fonts lists woff2 files only for browsers it knows.
	if req.URL.Host == googleFontsHost {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, media.MaxUpload+1))
	if err != nil {
		return nil, err
	}
	if len(b) > media.MaxUpload {
		return nil, errors.New("file too large")
	}
	return b, nil
}

type Options struct {
	Site           *site.Site
	PostsSection   string // folder for posts, default "posts"
	DownloadImages bool
	Fetch          Fetcher
	Progress       func(done, total int, label string)
}

type Report struct {
	Posts        int      `json:"posts"`
	Pages        int      `json:"pages"`
	Drafts       int      `json:"drafts"`
	Images       int      `json:"images"`
	Files        int      `json:"files"`
	MenuItems    int      `json:"menuItems"`
	Comments     int      `json:"comments"`
	FailedImages []string `json:"failedImages,omitempty"`
	FailedFiles  []string `json:"failedFiles,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
	// Icon is the WordPress Site Icon. Import doesn't write it, so the
	// caller can keep an icon the owner already chose.
	Icon *favicon.Icons `json:"-"`
	// Style is the old site's colors and fonts, applied by the caller the
	// same way as Icon.
	Style *appearance.Tokens `json:"-"`
	// CustomCSS is the old site's Additional CSS, commented out. The caller
	// appends it to Custom CSS.
	CustomCSS string `json:"-"`
	// ContactForms are the titles of pages that now have Pageroom's contact
	// form. The caller saves ContactEmail, where the old form sent messages, as
	// the site's contact email.
	ContactForms []string `json:"-"`
	ContactEmail string   `json:"-"`
}

type planned struct {
	item    *Item
	file    string // relative to site root
	url     string // new address
	section bool   // written as _index.md because it has child pages
}

// Import writes the export into the site. Existing files are never
// overwritten: a clash gets a new name.
//
//nolint:gocyclo // The import runs once per site in order, and the importer is well tested.
func Import(ctx context.Context, e *Export, opt Options) (*Report, error) {
	s := opt.Site
	if opt.PostsSection == "" {
		opt.PostsSection = "posts"
	}
	base, _ := url.Parse(e.Link)
	rep := &Report{CustomCSS: customCSSComments(e)}

	attachments := map[int]string{}
	blocks := map[int]string{}
	byID := map[int]*Item{}
	children := map[int][]*Item{}
	var content []*Item
	for i := range e.Items {
		it := &e.Items[i]
		byID[it.ID] = it
		switch it.Type {
		case "attachment":
			attachments[it.ID] = it.AttachURL
		case "wp_block":
			blocks[it.ID] = it.Content
		case "post", "page":
			if it.Status == "trash" || it.Status == "auto-draft" || it.Status == "inherit" {
				continue
			}
			content = append(content, it)
			if it.Type == "page" && it.Parent != 0 {
				children[it.Parent] = append(children[it.Parent], it)
			}
		}
	}

	// Work out where everything goes before writing anything.
	plans := map[int]*planned{}
	var order []*planned
	var place func(it *Item) *planned
	place = func(it *Item) *planned {
		if p := plans[it.ID]; p != nil {
			return p
		}
		slug := it.Slug
		if decoded, err := url.PathUnescape(slug); err == nil {
			slug = decoded
		}
		slug = fsutil.Slugify(firstNonEmpty(slug, it.Title, "post-"+strconv.Itoa(it.ID)))
		dir := path.Join(s.ContentDir, opt.PostsSection, slug)
		if it.Type == "page" {
			dir = path.Join(s.ContentDir, slug)
			if parent := byID[it.Parent]; parent != nil && parent.Type == "page" && it.Parent != it.ID {
				pp := place(parent)
				dir = path.Join(path.Dir(pp.file), slug)
			}
		}
		full := fsutil.UniquePath(filepath.Join(s.Root, filepath.FromSlash(dir)))
		rel, _ := filepath.Rel(s.Root, full)
		rel = filepath.ToSlash(rel)
		p := &planned{item: it, section: len(children[it.ID]) > 0}
		if p.section {
			p.file = rel + "/_index.md"
		} else {
			p.file = rel + "/index.md"
		}
		p.url = "/" + strings.TrimPrefix(rel, s.ContentDir+"/") + "/"
		plans[it.ID] = p
		order = append(order, p)
		// reserve the folder now so siblings with the same slug get another name
		_ = mkdirAll(filepath.Join(s.Root, filepath.FromSlash(rel)))
		return p
	}
	for _, it := range content {
		place(it)
	}

	cache := map[string][]byte{}
	for i, p := range order {
		select {
		case <-ctx.Done():
			return rep, ctx.Err()
		default:
		}
		if opt.Progress != nil {
			opt.Progress(i, len(order), p.item.Title)
		}
		if err := writeItem(ctx, e, p, opt, base, attachments, blocks, cache, rep); err != nil {
			return rep, fmt.Errorf("%q: %w", p.item.Title, err)
		}
	}

	if len(rep.ContactForms) > 0 {
		if err := opt.Site.AddContactForm(); err != nil {
			return rep, err
		}
	}
	if err := importMenus(e, plans, opt.Site, base, rep); err != nil {
		rep.Warnings = append(rep.Warnings, "Menus couldn't be imported: "+err.Error())
	}
	if opt.DownloadImages && base != nil {
		if page, err := opt.Fetch.Get(ctx, e.Link); err == nil {
			var stylesheets [][]byte
			var faces []FontFace
			for _, block := range styleBlock.FindAllSubmatch(page, -1) {
				faces = append(faces, FontFaces(string(block[2]), base)...)
			}
			for _, sheetURL := range StylesheetURLs(page, base) {
				if sheet, err := opt.Fetch.Get(ctx, sheetURL); err == nil {
					stylesheets = append(stylesheets, sheet)
					if sheetBase, err := url.Parse(sheetURL); err == nil {
						faces = append(faces, FontFaces(string(sheet), sheetBase)...)
					}
				}
			}
			for _, fontsURL := range GoogleFontsURLs(page, base) {
				if sheet, err := opt.Fetch.Get(ctx, fontsURL); err == nil {
					if sheetBase, err := url.Parse(fontsURL); err == nil {
						faces = append(faces, FontFaces(string(sheet), sheetBase)...)
					}
				}
			}
			var families fontFamilies
			rep.Style, families = readStyles(page, stylesheets...)
			rep.Style = importFonts(ctx, faces, families, base, opt, rep.Style)
			if err := importLogo(ctx, page, base, opt, cache, rep); err != nil {
				return rep, err
			}
		}
	}
	if u := siteIcon(e); u != "" && opt.DownloadImages {
		data, err := opt.Fetch.Get(ctx, u)
		if err == nil {
			rep.Icon, err = favicon.FromImage(data)
		}
		if err != nil {
			rep.FailedImages = append(rep.FailedImages, u)
		}
	}
	if opt.Progress != nil {
		opt.Progress(len(order), len(order), "")
	}
	return rep, nil
}

// importFonts copies the body and heading fonts from the old site and from
// Google Fonts into static/fonts/ and lists them in the style. A font that
// can't be downloaded stays as the built-in stack the style already has.
func importFonts(ctx context.Context, faces []FontFace, families fontFamilies, base *url.URL, opt Options, style *appearance.Tokens) *appearance.Tokens {
	bodyFamily := families.body
	headingFamily := cmp.Or(families.heading, families.body)
	downloaded := map[string]string{}
	var fonts []appearance.SiteFont
	for _, family := range uniq(bodyFamily, headingFamily) {
		font := appearance.SiteFont{Family: family}
		for _, face := range faces {
			if !strings.EqualFold(face.Family, family) {
				continue
			}
			link, err := url.Parse(face.URL)
			if err != nil || (link.Host != base.Host && link.Host != googleFontFilesHost) {
				continue
			}
			name, ok := downloaded[face.URL]
			if !ok {
				if name, err = downloadFile(ctx, face.URL, filepath.Join(opt.Site.Root, "static", "fonts"), opt, map[string][]byte{}); err != nil {
					continue
				}
				downloaded[face.URL] = name
			}
			font.Files = append(font.Files, appearance.FontFile{Path: "fonts/" + name, Weight: face.Weight, Style: face.Style, UnicodeRange: face.UnicodeRange})
		}
		if len(font.Files) > 0 {
			fonts = append(fonts, font)
		}
	}
	if len(fonts) == 0 {
		return style
	}

	result := appearance.Tokens{Radius: -1}
	if style != nil {
		result = *style
	}
	for index := range fonts {
		fonts[index].Fallback = fontFallback(fonts[index].Family, bodyFamily, result)
	}
	result.Fonts = fonts
	if slices.ContainsFunc(fonts, func(font appearance.SiteFont) bool { return font.Family == bodyFamily }) {
		result.BodyFont = bodyFamily
	}
	if slices.ContainsFunc(fonts, func(font appearance.SiteFont) bool { return font.Family == headingFamily }) {
		result.HeadingFont = headingFamily
	}
	if result.Validate() != nil {
		return style
	}
	return &result
}

// fontFallback is the built-in stack the style had for the role a family
// plays, so a font that fails to load looks close to the old one.
func fontFallback(family, bodyFamily string, style appearance.Tokens) string {
	if family == bodyFamily {
		return style.BodyFont
	}
	return cmp.Or(style.HeadingFont, style.BodyFont)
}

// importLogo downloads the homepage's logo into static/ and sets
// params.logo, only for Pageroom's starter themes and only if the site has no
// logo yet.
func importLogo(ctx context.Context, page []byte, base *url.URL, opt Options, cache map[string][]byte, rep *Report) error {
	s := opt.Site
	if !strings.HasPrefix(s.Config.Theme(), "pageroom-") || s.Logo() != "" {
		return nil
	}
	logoURL := LogoURL(page, base)
	if logoURL == "" {
		return nil
	}
	name, err := download(ctx, logoURL, filepath.Join(s.Root, "static"), opt, cache)
	if err != nil {
		rep.FailedImages = append(rep.FailedImages, logoURL)
		return nil
	}
	rep.Images++
	return s.SetLogo("/" + name)
}

//nolint:gocyclo // One pass over an item keeps the import steps in order, and the importer is well tested.
func writeItem(ctx context.Context, e *Export, p *planned, opt Options, base *url.URL, attachments, blocks map[int]string, cache map[string][]byte, rep *Report) error {
	it := p.item
	dirRel := path.Dir(p.file)
	dirAbs := filepath.Join(opt.Site.Root, filepath.FromSlash(dirRel))

	prepared := prepare(it.Content, attachments, blocks)
	local := map[string]string{}
	var featured string
	wanted := fileURLs(prepared)
	if id, err := strconv.Atoi(it.Meta["_thumbnail_id"]); err == nil {
		if u, ok := attachments[id]; ok {
			wanted = append(wanted, u)
			featured = u
		}
	}
	for _, u := range wanted {
		if !opt.DownloadImages || !sameSite(u, base, attachments) {
			continue
		}
		if isFile(u) {
			name, err := downloadFile(ctx, u, dirAbs, opt, cache)
			if err != nil {
				rep.FailedFiles = append(rep.FailedFiles, u)
				continue
			}
			local[u] = name
			rep.Files++
			continue
		}
		if name, ok := local[originalOf(u)]; ok { // another size of an image we already have
			local[u] = name
			continue
		}
		name, err := download(ctx, u, dirAbs, opt, cache)
		if err != nil {
			rep.FailedImages = append(rep.FailedImages, u)
			continue
		}
		local[u] = name
		local[originalOf(u)] = name
		rep.Images++
	}

	md, unknown, err := toMarkdown(prepared, it.Meta["footnotes"], local, base)
	if err != nil {
		return err
	}
	for _, name := range []string{"audio", "gist", "google-map", "pr-align", "pr-button", "pr-video"} {
		if strings.Contains(md, "{{< "+name+" ") {
			if err := opt.Site.AddShortcode(name); err != nil {
				return err
			}
		}
	}
	for _, host := range otherIframes(prepared) {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("“%s” shows something embedded from %s, which doesn't come over to the new site. Please check that page.", it.Title, host))
	}
	for _, m := range cf7SC.FindAllStringSubmatch(prepared, -1) {
		if !slices.Contains(rep.ContactForms, it.Title) {
			rep.ContactForms = append(rep.ContactForms, it.Title)
		}
		form := cf7Form(e, m[1])
		if form == nil {
			continue
		}
		if rep.ContactEmail == "" {
			rep.ContactEmail = cf7Recipient(form)
		}
		if extra := cf7ExtraFields(form); len(extra) > 0 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("The contact form on “%s” asks only for a name, email and message. The WordPress form also asked for: %s.", it.Title, strings.Join(extra, ", ")))
		}
	}
	for _, sc := range unknown {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("“%s” uses the WordPress plugin shortcode [%s], which doesn't work on the new site. Please check that page.", it.Title, sc))
	}

	doc, _ := frontmatter.Parse(nil)
	f := opt.Site.DefaultFormat()
	set := func(k string, v any) { _ = doc.Set(k, v, f) }
	set("title", firstNonEmpty(it.Title, "Untitled"))
	if !it.Date.IsZero() {
		set("date", it.Date)
	}
	if it.Status != "publish" && it.Status != "future" {
		set("draft", true)
		rep.Drafts++
	}
	if it.Type == "page" && it.MenuOrder != 0 {
		set("weight", it.MenuOrder)
	}
	if d := plainText(it.Excerpt); d != "" {
		set("description", d)
	}
	if a := e.Authors[it.Creator]; a != "" {
		set("author", a)
	}
	var cats, tags []string
	for _, t := range it.Terms {
		switch t.Domain {
		case "category":
			if t.Nicename != "uncategorized" {
				cats = append(cats, t.Name)
			}
		case "post_tag":
			tags = append(tags, t.Name)
		}
	}
	if len(cats) > 0 {
		set("categories", cats)
	}
	if len(tags) > 0 {
		set("tags", tags)
	}
	if name, ok := local[featured]; ok && featured != "" {
		set("image", name)
	}
	if old := oldPath(it.Link, base); old != "" && old != p.url && it.Status == "publish" {
		set("aliases", []string{old})
	}
	doc.SetBody([]byte(md))
	out, err := doc.Bytes()
	if err != nil {
		return err
	}
	full := filepath.Join(opt.Site.Root, filepath.FromSlash(p.file))
	if err := fsutil.CreateExclusive(full, bytes.NewReader(out)); err != nil {
		return err
	}
	if it.Type == "post" {
		rep.Posts++
	} else {
		rep.Pages++
	}
	rep.Comments += it.Comments
	return nil
}

// siteIcon is the newest image cropped for WordPress's Site Icon setting.
// The export doesn't say which one is in use, and the newest is the likeliest.
func siteIcon(e *Export) string {
	var newest *Item
	for i := range e.Items {
		it := &e.Items[i]
		if it.Type == "attachment" && it.Meta["_wp_attachment_context"] == "site-icon" && (newest == nil || it.ID > newest.ID) {
			newest = it
		}
	}
	if newest == nil {
		return ""
	}
	return newest.AttachURL
}

// sameSite limits downloads to the WordPress site's own uploads, and to the
// attachment URLs listed in the export.
func sameSite(u string, base *url.URL, attachments map[int]string) bool {
	for _, a := range attachments {
		if a == u || originalOf(u) == a {
			return true
		}
	}
	pu, err := url.Parse(u)
	if err != nil || base == nil {
		return false
	}
	return strings.TrimPrefix(pu.Hostname(), "www.") == strings.TrimPrefix(base.Hostname(), "www.") && strings.Contains(pu.Path, "/wp-content/uploads/")
}

// downloadFile copies a file such as a PDF into the page's folder as it is.
func downloadFile(ctx context.Context, u, dir string, opt Options, cache map[string][]byte) (string, error) {
	data, ok := cache[u]
	if !ok {
		var err error
		if data, err = opt.Fetch.Get(ctx, u); err != nil {
			return "", err
		}
		cache[u] = data
	}
	if err := mkdirAll(dir); err != nil {
		return "", err
	}
	name := path.Base(urlPath(u))
	ext := strings.ToLower(path.Ext(name))
	target := fsutil.UniquePath(filepath.Join(dir, fsutil.Slugify(strings.TrimSuffix(name, path.Ext(name)))+ext))
	if err := fsutil.CreateExclusive(target, bytes.NewReader(data)); err != nil {
		return "", err
	}
	return filepath.Base(target), nil
}

func download(ctx context.Context, u, dir string, opt Options, cache map[string][]byte) (string, error) {
	var data []byte
	var err error
	// Prefer the full-size original over a resized variant.
	for _, candidate := range uniq(originalOf(u), u) {
		if b, ok := cache[candidate]; ok {
			data, err = b, nil
			break
		}
		data, err = opt.Fetch.Get(ctx, candidate)
		if err == nil {
			cache[candidate] = data
			break
		}
	}
	if err != nil {
		return "", err
	}
	processed, ext, _, err := media.Process(data, media.Options{MaxWidth: opt.Site.Prefs.Media.MaxWidth})
	if err != nil {
		return "", err
	}
	pu, _ := url.Parse(originalOf(u))
	stem := strings.TrimSuffix(path.Base(pu.Path), path.Ext(pu.Path))
	if err := mkdirAll(dir); err != nil {
		return "", err
	}
	target := fsutil.UniquePath(filepath.Join(dir, fsutil.Slugify(stem)+ext))
	if err := fsutil.CreateExclusive(target, bytes.NewReader(processed)); err != nil {
		return "", err
	}
	return filepath.Base(target), nil
}

func oldPath(link string, base *url.URL) string {
	u, err := url.Parse(link)
	if err != nil || u.Path == "" || u.Path == "/" || u.RawQuery != "" {
		return ""
	}
	if base != nil && base.Host != "" && strings.TrimPrefix(u.Hostname(), "www.") != strings.TrimPrefix(base.Hostname(), "www.") {
		return ""
	}
	p := u.Path
	if !strings.HasSuffix(p, "/") && path.Ext(p) == "" {
		p += "/"
	}
	return p
}

// importMenus recreates WordPress navigation menus as Hugo menus.
func importMenus(e *Export, plans map[int]*planned, s *site.Site, base *url.URL, rep *Report) error {
	type entry struct {
		item  *Item
		menu  string
		order int
	}
	var entries []entry
	for i := range e.Items {
		it := &e.Items[i]
		if it.Type != "nav_menu_item" {
			continue
		}
		for _, t := range it.Terms {
			if t.Domain == "nav_menu" {
				entries = append(entries, entry{item: it, menu: t.Nicename, order: it.MenuOrder})
			}
		}
	}
	if len(entries) == 0 {
		return nil
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].order < entries[j].order })
	first := entries[0].menu
	menus := map[string][]site.MenuItem{}
	var names []string
	for _, en := range entries {
		it := en.item
		mi := site.MenuItem{ID: "wp-" + strconv.Itoa(it.ID), Name: strings.TrimSpace(it.Title), Source: "config"}
		if p := it.Meta["_menu_item_menu_item_parent"]; p != "" && p != "0" {
			mi.Parent = "wp-" + p
		}
		switch it.Meta["_menu_item_type"] {
		case "post_type":
			id, _ := strconv.Atoi(it.Meta["_menu_item_object_id"])
			p := plans[id]
			if p == nil {
				continue
			}
			mi.PageRef = strings.TrimSuffix(p.url, "/")
			if mi.PageRef == "" {
				mi.PageRef = "/"
			}
			if mi.Name == "" {
				mi.Name = p.item.Title
			}
		case "custom":
			mi.URL = it.Meta["_menu_item_url"]
			if base != nil {
				if u, err := url.Parse(mi.URL); err == nil && strings.TrimPrefix(u.Hostname(), "www.") == strings.TrimPrefix(base.Hostname(), "www.") {
					mi.URL = firstNonEmpty(u.Path, "/")
				}
			}
		case "taxonomy":
			id, _ := strconv.Atoi(it.Meta["_menu_item_object_id"])
			term, ok := e.Terms[id]
			taxonomy := map[string]string{"category": "categories", "post_tag": "tags"}[term.Domain]
			if !ok || taxonomy == "" || term.Domain != it.Meta["_menu_item_object"] || term.Nicename == "uncategorized" {
				continue
			}
			mi.PageRef = "/" + taxonomy + "/" + term.Name
			if mi.Name == "" {
				mi.Name = term.Name
			}
		default:
			continue
		}
		if mi.Name == "" {
			continue
		}
		name := en.menu
		if name == first {
			name = "main"
		}
		if _, ok := menus[name]; !ok {
			names = append(names, name)
		}
		menus[name] = append(menus[name], mi)
		rep.MenuItems++
	}
	for _, name := range names {
		if err := s.SaveMenu(site.Menu{Name: name, Items: menus[name]}); err != nil {
			return err
		}
		if err := s.Reload(); err != nil {
			return err
		}
	}
	return nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func uniq(v ...string) []string {
	var out []string
	for _, s := range v {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}

var htmlTagStart = regexp.MustCompile(`(?i)<(/?(?:style|script))`)

// customCSSComments turns each theme's Additional CSS into a comment.
// WordPress keeps one per theme and the export doesn't say which is active,
// so none is switched on.
func customCSSComments(e *Export) string {
	var blocks []string
	for _, it := range e.Items {
		css := strings.TrimSpace(it.Content)
		if it.Type != "custom_css" || it.Status != "publish" || css == "" {
			continue
		}
		theme := firstNonEmpty(it.Slug, it.Title, "unknown")
		blocks = append(blocks, "/* Additional CSS from the WordPress theme "+commentSafe(theme)+", switched off:\n\n"+commentSafe(css)+"\n*/")
	}
	return strings.Join(blocks, "\n\n")
}

// commentSafe stops text from ending its comment early, and from tripping the
// check that keeps HTML out of Custom CSS.
func commentSafe(text string) string {
	return htmlTagStart.ReplaceAllString(strings.ReplaceAll(text, "*/", "* /"), "< $1")
}
