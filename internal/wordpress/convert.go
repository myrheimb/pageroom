package wordpress

import (
	"encoding/json"
	"fmt"
	"html"
	"net/mail"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"

	"github.com/myrheimb/pageroom/internal/media"
	"github.com/myrheimb/pageroom/internal/site"
)

var (
	blockComment = regexp.MustCompile(`<!--\s*/?wp:[\s\S]*?-->`)
	reusableRef  = regexp.MustCompile(`<!--\s*wp:block\s+\{[^}]*?"ref":\s*(\d+)[^}]*\}\s*/-->`)
	figcaption   = regexp.MustCompile(`(?is)<figcaption[^>]*>(.*?)</figcaption>`)
	captionSC    = regexp.MustCompile(`(?s)\[caption[^\]]*\](.*?)\[/caption\]`)
	embedSC      = regexp.MustCompile(`(?s)\[embed[^\]]*\](.*?)\[/embed\]`)
	gallerySC    = regexp.MustCompile(`\[gallery([^\]]*)\]`)
	galleryIDs   = regexp.MustCompile(`ids=["']?([\d,\s]+)`)
	embedWrapper = regexp.MustCompile(`(?s)<figure[^>]*wp-block-embed[^>]*>\s*<div[^>]*wp-block-embed__wrapper[^>]*>\s*(\S+?)\s*</div>.*?</figure>`)
	youtubeURL   = regexp.MustCompile(`^https?://(?:www\.|m\.)?(?:youtube(?:-nocookie)?\.com/(?:watch\?(?:.*&)?v=|shorts/|embed/)|youtu\.be/)([\w-]{6,})`)
	vimeoURL     = regexp.MustCompile(`^https?://(?:www\.|player\.)?vimeo\.com/(?:video/)?(\d+)`)
	googleMapURL = regexp.MustCompile(`^https://(?:www\.|maps\.)?google\.[a-z.]+/maps(?:/d)?(?:/embed\?|\?.*\boutput=embed)`)
	gistURL      = regexp.MustCompile(`^https?://gist\.github\.com/((?:[\w-]+/)?[0-9a-f]+)(?:\.js)?/?(?:[?#].*)?$`)
	gistPath     = regexp.MustCompile(`^(?:[\w-]+/)?[0-9a-f]+$`)
	moreTag      = regexp.MustCompile(`<!--more\b.*?-->`)
	audioSC      = regexp.MustCompile(`\[audio(\s[^\]]*)?\](?:\s*\[/audio\])?`)
	audioSrc     = regexp.MustCompile(`\b(?:src|mp3|m4a|ogg|wav)=["']([^"']+)["']`)
	videoSC      = regexp.MustCompile(`\[video(\s[^\]]*)?\](?:\s*\[/video\])?`)
	videoSrc     = regexp.MustCompile(`\b(?:src|mp4|m4v|webm|ogv)=["']([^"']+)["']`)
	gistSC       = regexp.MustCompile(`\[gist(\s[^\]]*)?\](?:([^\[]*)\[/gist\])?`)
	iframeSrc    = regexp.MustCompile(`(?i)<iframe\b[^>]*?\bsrc=["']([^"']+)["']`)
	bareURLLine  = regexp.MustCompile(`(?m)^[ \t]*(https?://\S+)[ \t]*$`)
	blockTag     = regexp.MustCompile(`(?i)<(p|div|h[1-6]|ul|ol|blockquote|figure|table|pre)\b`)
	imgSrc       = regexp.MustCompile(`(?i)<img\b[^>]*?\bsrc=["']([^"']+)["']`)
	mediaSrc     = regexp.MustCompile(`(?i)<(?:audio|video|source)\b[^>]*?\bsrc=["']([^"']+)["']`)
	linkHref     = regexp.MustCompile(`(?i)<a\b[^>]*?\bhref=["']([^"']+)["']`)
	otherSC      = regexp.MustCompile(`\[([a-z][\w-]*)(?:\s[^\]]*)?\]`)
	cf7SC        = regexp.MustCompile(`\\?\[contact-form-7(\s[^\]]*)?\]`)
	cf7ID        = regexp.MustCompile(`\bid=["']?(\w+)`)
	cf7Field     = regexp.MustCompile(`\[(\w+)\*?\s+([\w-]+)`)
	cf7MailTo    = regexp.MustCompile(`"recipient";s:\d+:"([^"]*)"`)
)

// fileExts are the files besides images that come over with the pages
// linking to them.
var fileExts = map[string]bool{
	".csv": true, ".doc": true, ".docx": true, ".epub": true, ".m4a": true, ".m4v": true, ".mov": true, ".mp3": true,
	".mp4": true, ".odp": true, ".ods": true, ".odt": true, ".oga": true, ".ogg": true, ".ogv": true, ".pdf": true,
	".ppt": true, ".pptx": true, ".rtf": true, ".txt": true, ".wav": true, ".webm": true, ".xls": true, ".xlsx": true, ".zip": true,
}

var conv = newConverter()

func newConverter() *converter.Converter {
	markdown := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(commonmark.WithBulletListMarker("-"), commonmark.WithEmDelimiter("_")),
		table.NewTablePlugin(),
		strikethrough.NewStrikethroughPlugin(),
	))
	// Hugo ends a page's summary at <!--more-->, like WordPress does.
	markdown.Register.RendererFor("#comment", converter.TagTypeBlock, func(_ converter.Context, writer converter.Writer, node *nethtml.Node) converter.RenderStatus {
		if strings.HasPrefix(strings.TrimSpace(node.Data), "more") {
			_, _ = writer.WriteString("\n\n<!--more-->\n\n")
		}
		return converter.RenderSuccess
	}, converter.PriorityEarly)
	markdown.Register.RendererFor("iframe", converter.TagTypeBlock, func(_ converter.Context, writer converter.Writer, node *nethtml.Node) converter.RenderStatus {
		_, _ = writer.WriteString("\n\n" + embedShortcode(attr(node, "src"), attr(node, "title")) + "\n\n")
		return converter.RenderSuccess
	}, converter.PriorityEarly)
	markdown.Register.RendererFor("script", converter.TagTypeBlock, func(_ converter.Context, writer converter.Writer, node *nethtml.Node) converter.RenderStatus {
		if gist := gistURL.FindStringSubmatch(attr(node, "src")); gist != nil {
			_, _ = writer.WriteString("\n\n" + gistShortcode(gist[1]) + "\n\n")
		}
		return converter.RenderSuccess
	}, converter.PriorityEarly)
	for _, tag := range []string{"audio", "video"} {
		markdown.Register.RendererFor(tag, converter.TagTypeBlock, func(_ converter.Context, writer converter.Writer, node *nethtml.Node) converter.RenderStatus {
			src := attr(node, "src")
			for child := node.FirstChild; child != nil && src == ""; child = child.NextSibling {
				if child.Type == nethtml.ElementNode && child.Data == "source" {
					src = attr(child, "src")
				}
			}
			if src != "" && !strings.Contains(src, `"`) {
				name := tag
				if tag == "video" {
					name = "pr-video"
				}
				_, _ = fmt.Fprintf(writer, "\n\n{{< %s src=\"%s\" >}}\n\n", name, src)
			}
			return converter.RenderSuccess
		}, converter.PriorityEarly)
	}
	return markdown
}

func attr(node *nethtml.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

// embedShortcode is the Hugo shortcode showing what an iframe showed, or ""
// when there's none.
func embedShortcode(src, title string) string {
	if strings.HasPrefix(src, "//") {
		src = "https:" + src
	}
	if video := youtubeURL.FindStringSubmatch(src); video != nil {
		return `{{< pr-video id="` + video[1] + `" >}}`
	}
	if video := vimeoURL.FindStringSubmatch(src); video != nil {
		return `{{< pr-video vimeo="` + video[1] + `" >}}`
	}
	if googleMapURL.MatchString(src) && !strings.Contains(src, `"`) {
		if title = strings.ReplaceAll(title, `"`, ""); title != "" {
			return fmt.Sprintf(`{{< google-map src="%s" title="%s" >}}`, src, title)
		}
		return fmt.Sprintf(`{{< google-map src="%s" >}}`, src)
	}
	return ""
}

// gistShortcode takes "user/id" or a bare id, as Jetpack allows.
func gistShortcode(gist string) string {
	return "{{< gist " + strings.ReplaceAll(gist, "/", " ") + " >}}"
}

// gistScript turns Jetpack's [gist] shortcode into the script GitHub gives
// for embedding a gist. It names the gist by address or by id.
func gistScript(match []string) string {
	ref := strings.TrimSpace(match[2])
	if ref == "" {
		ref = strings.Trim(strings.TrimPrefix(strings.TrimSpace(match[1]), "id="), `"'`)
	}
	if gist := gistURL.FindStringSubmatch(ref); gist != nil {
		ref = gist[1]
	}
	if !gistPath.MatchString(ref) {
		return match[0]
	}
	return `<script src="https://gist.github.com/` + ref + `.js"></script>`
}

// expandBlocks puts in the content of the reusable blocks a page uses. Like
// WordPress, it leaves out a block placed inside itself.
func expandBlocks(content string, blocks map[int]string, inside map[int]bool) string {
	return reusableRef.ReplaceAllStringFunc(content, func(reference string) string {
		id, _ := strconv.Atoi(reusableRef.FindStringSubmatch(reference)[1])
		if inside[id] {
			return ""
		}
		inside[id] = true
		defer delete(inside, id)
		return expandBlocks(blocks[id], blocks, inside)
	})
}

// prepare cleans WordPress markup so the HTML converts well. attachments maps
// attachment IDs to URLs (for [gallery ids=...]) and blocks maps reusable
// block IDs to their content.
func prepare(content string, attachments map[int]string, blocks map[int]string) string {
	s := expandBlocks(content, blocks, map[int]bool{})
	s = blockComment.ReplaceAllString(s, "")
	s = moreTag.ReplaceAllString(s, "\n\n<!--more-->\n\n")
	s = audioSC.ReplaceAllStringFunc(s, func(shortcode string) string {
		if src := audioSrc.FindStringSubmatch(shortcode); src != nil {
			return `<audio src="` + src[1] + `"></audio>`
		}
		return shortcode
	})
	s = videoSC.ReplaceAllStringFunc(s, func(shortcode string) string {
		if src := videoSrc.FindStringSubmatch(shortcode); src != nil {
			return `<video src="` + src[1] + `"></video>`
		}
		return shortcode
	})
	s = gistSC.ReplaceAllStringFunc(s, func(shortcode string) string { return gistScript(gistSC.FindStringSubmatch(shortcode)) })
	s = embedWrapper.ReplaceAllString(s, "\n\n$1\n\n")
	s = embedSC.ReplaceAllString(s, "\n\n$1\n\n")
	s = captionSC.ReplaceAllStringFunc(s, func(m string) string {
		inner := captionSC.FindStringSubmatch(m)[1]
		img := ""
		if loc := regexp.MustCompile(`(?is)(<a[^>]*>\s*)?<img[^>]*>(\s*</a>)?`).FindStringIndex(inner); loc != nil {
			img = inner[loc[0]:loc[1]]
			inner = inner[:loc[0]] + inner[loc[1]:]
		}
		return "<figure>" + img + "<figcaption>" + strings.TrimSpace(inner) + "</figcaption></figure>"
	})
	s = figcaption.ReplaceAllString(s, "<p><em>$1</em></p>")
	s = gallerySC.ReplaceAllStringFunc(s, func(m string) string {
		ids := galleryIDs.FindStringSubmatch(m)
		if ids == nil {
			return m
		}
		var b strings.Builder
		for _, id := range strings.Split(ids[1], ",") {
			var n int
			_, _ = fmt.Sscan(strings.TrimSpace(id), &n)
			if u, ok := attachments[n]; ok {
				fmt.Fprintf(&b, `<p><img src="%s" alt=""></p>`, html.EscapeString(u))
			}
		}
		return b.String()
	})
	// Classic-editor posts are stored without <p>: WordPress adds them on
	// display ("wpautop"). Do the same before converting.
	if !blockTag.MatchString(s) {
		var paras []string
		for _, p := range regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(s), -1) {
			if p = strings.TrimSpace(p); p != "" {
				paras = append(paras, "<p>"+strings.ReplaceAll(p, "\n", "<br>\n")+"</p>")
			}
		}
		s = strings.Join(paras, "\n")
	}
	return s
}

// fileURLs lists the images and files the content shows or links to.
func fileURLs(content string) []string {
	seen := map[string]bool{}
	var out []string
	for _, re := range []*regexp.Regexp{imgSrc, mediaSrc, linkHref} {
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			u := html.UnescapeString(m[1])
			if re != imgSrc && !media.IsImage(urlPath(u)) && !isFile(u) {
				continue
			}
			if !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
	}
	return out
}

// isFile tells files, which are copied as they are, from images, which are
// processed.
func isFile(u string) bool { return fileExts[strings.ToLower(path.Ext(urlPath(u)))] }

func urlPath(u string) string {
	if parsed, err := url.Parse(u); err == nil {
		return parsed.Path
	}
	return u
}

// otherIframes names the sites of embedded frames Pageroom has no shortcode for.
func otherIframes(content string) []string {
	var hosts []string
	for _, match := range iframeSrc.FindAllStringSubmatch(content, -1) {
		src := html.UnescapeString(strings.TrimSpace(match[1]))
		if embedShortcode(src, "") != "" {
			continue
		}
		host := src
		if parsed, err := url.Parse(src); err == nil && parsed.Host != "" {
			host = parsed.Hostname()
		}
		if !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

type footnote struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// addFootnotes turns the block editor's footnotes, which it keeps beside
// the content, into Markdown footnotes.
func addFootnotes(md, stored string) string {
	var notes []footnote
	if json.Unmarshal([]byte(stored), &notes) != nil || len(notes) == 0 {
		return md
	}
	var defs []string
	for index, note := range notes {
		label := fmt.Sprintf("[^%d]", index+1)
		ref := regexp.MustCompile(`\[[^\]]*\]\(#` + regexp.QuoteMeta(note.ID) + `\)`)
		md = ref.ReplaceAllLiteralString(md, label)
		text, err := conv.ConvertString(note.Content)
		if err != nil {
			text = plainText(note.Content)
		}
		defs = append(defs, label+": "+strings.Join(strings.Fields(text), " "))
	}
	return strings.TrimRight(md, "\n") + "\n\n" + strings.Join(defs, "\n") + "\n"
}

// toMarkdown converts prepared HTML and rewrites URLs: downloaded files get
// local names, and links to the old site become site-relative.
func toMarkdown(prepared, footnotes string, local map[string]string, oldBase *url.URL) (string, []string, error) {
	md, err := conv.ConvertString(prepared)
	if err != nil {
		return "", nil, err
	}
	md = addFootnotes(md, footnotes)
	// Longest URLs first so a sized variant isn't partially replaced.
	for _, from := range sortedByLength(local) {
		md = strings.ReplaceAll(md, from, local[from])
	}
	if oldBase != nil && oldBase.Host != "" {
		// Links to the old site's pages become site-relative, and aliases keep them
		// working. Files in wp-content that failed to download keep their full
		// address and load from the old site.
		host := regexp.QuoteMeta(strings.TrimPrefix(oldBase.Hostname(), "www."))
		re := regexp.MustCompile(`\]\(https?://(?:www\.)?` + host + `(/[^)\s]*)`)
		md = re.ReplaceAllStringFunc(md, func(m string) string {
			p := re.FindStringSubmatch(m)[1]
			if strings.HasPrefix(p, "/wp-content/") {
				return m
			}
			return "](" + p
		})
	}
	// Video links alone on a line were auto-embedded by WordPress.
	md = bareURLLine.ReplaceAllStringFunc(md, func(line string) string {
		u := strings.TrimSpace(line)
		if m := youtubeURL.FindStringSubmatch(u); m != nil {
			return `{{< pr-video id="` + m[1] + `" >}}`
		}
		if m := vimeoURL.FindStringSubmatch(u); m != nil {
			return `{{< pr-video vimeo="` + m[1] + `" >}}`
		}
		if gist := gistURL.FindStringSubmatch(u); gist != nil {
			return gistShortcode(gist[1])
		}
		return line
	})
	md = cf7SC.ReplaceAllString(md, site.ContactFormShortcode)
	// Leftover plugin shortcodes won't work in Hugo: report them.
	var warnings []string
	seen := map[string]bool{}
	for _, m := range otherSC.FindAllStringSubmatch(md, -1) {
		name := m[1]
		if !seen[name] && !isMarkdownLinkText(md, m[0]) {
			seen[name] = true
			warnings = append(warnings, name)
		}
	}
	return strings.TrimSpace(md) + "\n", warnings, nil
}

// cf7Form finds the Contact Form 7 form a shortcode's attributes name:
// by post ID, or by the first 7 characters of its hash in newer versions.
func cf7Form(e *Export, attrs string) *Item {
	m := cf7ID.FindStringSubmatch(attrs)
	if m == nil {
		return nil
	}
	for i := range e.Items {
		it := &e.Items[i]
		if it.Type == "wpcf7_contact_form" && (strconv.Itoa(it.ID) == m[1] || len(m[1]) >= 7 && strings.HasPrefix(it.Meta["_hash"], m[1])) {
			return it
		}
	}
	return nil
}

// cf7Recipient is the address a form's messages went to. WordPress
// placeholders like [_site_admin_email] can't be resolved from an export.
func cf7Recipient(form *Item) string {
	m := cf7MailTo.FindStringSubmatch(form.Meta["_mail"])
	if m == nil {
		return ""
	}
	if a, err := mail.ParseAddress(m[1]); err == nil && a.Address == m[1] {
		return m[1]
	}
	return ""
}

// cf7ExtraFields names a form's fields beyond the ones Pageroom's contact
// form has. The subject is left out: the message covers it.
func cf7ExtraFields(form *Item) []string {
	skip := map[string]bool{"your-name": true, "your-email": true, "your-subject": true, "your-message": true}
	var extra []string
	for _, m := range cf7Field.FindAllStringSubmatch(form.Meta["_form"], -1) {
		if m[1] != "submit" && m[1] != "hidden" && !skip[m[2]] {
			skip[m[2]] = true
			extra = append(extra, m[2])
		}
	}
	return extra
}

func isMarkdownLinkText(md, token string) bool {
	i := strings.Index(md, token)
	return i >= 0 && i+len(token) < len(md) && md[i+len(token)] == '('
}

func sortedByLength(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && len(keys[j]) > len(keys[j-1]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

var sizeSuffix = regexp.MustCompile(`-\d+x\d+(\.\w+)$`)

// originalOf maps "photo-1024x768.jpg" to "photo.jpg", the full-size upload.
func originalOf(u string) string { return sizeSuffix.ReplaceAllString(u, "$1") }

// plainText turns an excerpt into a one-line description.
func plainText(s string) string {
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}
