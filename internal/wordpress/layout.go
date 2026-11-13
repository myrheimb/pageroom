package wordpress

import (
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// markLayout puts text markers around the aligned blocks and the columns and
// in place of the buttons in prepared HTML. toMarkdown turns the markers into
// pr-align, pr-columns and pr-button shortcodes once the blocks are Markdown.
func markLayout(content string) string {
	if !strings.Contains(content, "align") && !strings.Contains(content, "wp-block-button") && !strings.Contains(content, "wp-block-column") && !strings.Contains(content, "wp-block-media-text") && !strings.Contains(content, "wp-block-cover") {
		return content
	}
	body := &nethtml.Node{Type: nethtml.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := nethtml.ParseFragment(strings.NewReader(content), body)
	if err != nil {
		return content
	}
	for _, node := range nodes {
		body.AppendChild(node)
	}
	markNode(body)

	var out strings.Builder
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := nethtml.Render(&out, child); err != nil {
			return content
		}
	}
	return out.String()
}

func markNode(node *nethtml.Node) {
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == nethtml.ElementNode {
			markElement(child)
		}
		child = next
	}
}

func markElement(el *nethtml.Node) {
	if hasClass(el, "wp-block-buttons") {
		markButtons(el)
		return
	}
	if hasClass(el, "wp-block-columns") {
		markColumns(el)
		return
	}
	if hasClass(el, "wp-block-media-text") {
		markMediaText(el)
		return
	}
	if hasClass(el, "wp-block-cover") && markCover(el) {
		return
	}
	align := alignOf(el)
	switch {
	case align != "" && isTextBlock(el) && !insideContent(el):
		wrapAligned(el, align)
		return
	case el.Data == "img" && alignOf(el) != "" && !insideContent(el):
		markImage(el, alignOf(el))
		return
	case el.Data == "figure" && align != "" && !insideContent(el):
		if img := firstElement(el, "img"); img != nil && img.Parent == el {
			markImage(img, align)
		}
		return
	}
	markNode(el)
}

func markImage(img *nethtml.Node, align string) {
	parent := img.Parent
	if parent.Data == "a" {
		return
	}
	if parent.Data == "p" && onlyChild(parent) {
		wrapAligned(parent, align)
		return
	}
	paragraph := &nethtml.Node{Type: nethtml.ElementNode, Data: "p"}
	parent.InsertBefore(paragraph, img)
	parent.RemoveChild(img)
	paragraph.AppendChild(img)
	wrapAligned(paragraph, align)
}

func wrapAligned(el *nethtml.Node, align string) {
	el.Parent.InsertBefore(markerParagraph("PRALIGNSTART"+align), el)
	if el.NextSibling != nil {
		el.Parent.InsertBefore(markerParagraph("PRALIGNEND"), el.NextSibling)
	} else {
		el.Parent.AppendChild(markerParagraph("PRALIGNEND"))
	}
}

func markButtons(row *nethtml.Node) {
	align := ""
	switch {
	case hasClass(row, "is-content-justification-center"):
		align = "center"
	case hasClass(row, "is-content-justification-right"):
		align = "right"
	}
	for _, link := range allElements(row, "a") {
		if !hasClass(link, "wp-block-button__link") {
			continue
		}
		shortcode := buttonShortcode(attr(link, "href"), plainText(renderChildren(link)), align)
		if shortcode == "" {
			continue
		}
		button := link
		for button.Parent != row && !hasClass(button, "wp-block-button") && button.Parent != nil {
			button = button.Parent
		}
		if button.Parent != row {
			button = link
		}
		button.Parent.InsertBefore(markerParagraph("PRBUTTON"+hex.EncodeToString([]byte(shortcode))), button)
		button.Parent.RemoveChild(button)
	}
	for row.FirstChild != nil {
		child := row.FirstChild
		row.RemoveChild(child)
		row.Parent.InsertBefore(child, row)
	}
	row.Parent.RemoveChild(row)
}

func buttonShortcode(link, text, align string) string {
	if link == "" || text == "" || strings.ContainsAny(link+text, "\"\\") || strings.Contains(text, "{{") || strings.Contains(link, "{{") {
		return ""
	}
	if align != "" {
		align = fmt.Sprintf(` align="%s"`, align)
	}
	return fmt.Sprintf(`{{< pr-button link="%s"%s >}}%s{{< /pr-button >}}`, link, align, text)
}

func markerParagraph(text string) *nethtml.Node {
	paragraph := &nethtml.Node{Type: nethtml.ElementNode, Data: "p"}
	paragraph.AppendChild(&nethtml.Node{Type: nethtml.TextNode, Data: text})
	return paragraph
}

func alignOf(el *nethtml.Node) string {
	switch {
	case hasClass(el, "aligncenter"), hasClass(el, "has-text-align-center"):
		return "center"
	case hasClass(el, "alignright"), hasClass(el, "has-text-align-right"):
		return "right"
	}
	return ""
}

func isTextBlock(el *nethtml.Node) bool {
	switch el.Data {
	case "p", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote":
		return true
	}
	return false
}

// insideContent tells blocks in lists, tables and quotes, where the editor
// can't align a block on its own.
func insideContent(el *nethtml.Node) bool {
	for parent := el.Parent; parent != nil; parent = parent.Parent {
		switch parent.Data {
		case "li", "td", "th", "table", "blockquote", "a":
			return true
		}
	}
	return false
}

func onlyChild(el *nethtml.Node) bool {
	count := 0
	for child := el.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.ElementNode || strings.TrimSpace(child.Data) != "" {
			count++
		}
	}
	return count == 1
}

func hasClass(el *nethtml.Node, class string) bool {
	for _, name := range strings.Fields(attr(el, "class")) {
		if name == class {
			return true
		}
	}
	return false
}

func firstElement(el *nethtml.Node, tag string) *nethtml.Node {
	if found := allElements(el, tag); len(found) > 0 {
		return found[0]
	}
	return nil
}

func allElements(el *nethtml.Node, tag string) []*nethtml.Node {
	var found []*nethtml.Node
	for child := el.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != nethtml.ElementNode {
			continue
		}
		if child.Data == tag {
			found = append(found, child)
		}
		found = append(found, allElements(child, tag)...)
	}
	return found
}

func renderChildren(el *nethtml.Node) string {
	var out strings.Builder
	for child := el.FirstChild; child != nil; child = child.NextSibling {
		_ = nethtml.Render(&out, child)
	}
	return out.String()
}

var flexBasis = regexp.MustCompile(`flex-basis:\s*([\d.]+)%`)

// markColumns replaces a wp:columns block with markers around its columns
// when the editor's columns can hold it. Otherwise its content stays as it is.
func markColumns(row *nethtml.Node) {
	var columns []*nethtml.Node
	fits := true
	for child := row.FirstChild; child != nil; child = child.NextSibling {
		switch {
		case child.Type == nethtml.ElementNode && hasClass(child, "wp-block-column"):
			columns = append(columns, child)
		case child.Type == nethtml.ElementNode || child.Type == nethtml.TextNode && strings.TrimSpace(child.Data) != "":
			fits = false
		}
	}
	contents := make([][]*nethtml.Node, len(columns))
	for index, column := range columns {
		stackColumns(column)
		markNode(column)
		for child := column.FirstChild; child != nil; child = child.NextSibling {
			contents[index] = append(contents[index], child)
		}
	}
	layout, ok := columnsLayout(columns)
	if !fits || !ok {
		return
	}
	for _, content := range contents {
		if !fitsColumn(content) {
			return
		}
	}
	valign := verticalAlign(row)
	if valign == "" {
		valign = verticalAlign(columns...)
	}
	replaceWithColumns(row, layout, valign, contents)
}

// stackColumns replaces each wp:columns in a column with its columns'
// content, one after the other, since the editor can't nest columns.
func stackColumns(column *nethtml.Node) {
	for child := column.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == nethtml.ElementNode && hasClass(child, "wp-block-columns") {
			for inner := child.FirstChild; inner != nil; inner = child.FirstChild {
				child.RemoveChild(inner)
				if inner.Type == nethtml.ElementNode && hasClass(inner, "wp-block-column") {
					stackColumns(inner)
					for inner.FirstChild != nil {
						node := inner.FirstChild
						inner.RemoveChild(node)
						column.InsertBefore(node, child)
					}
				} else {
					column.InsertBefore(inner, child)
				}
			}
			column.RemoveChild(child)
		}
		child = next
	}
}

// markMediaText turns a wp:media-text block into two columns with the media
// on its side.
func markMediaText(block *nethtml.Node) {
	var media *nethtml.Node
	var content []*nethtml.Node
	for child := block.FirstChild; child != nil; child = child.NextSibling {
		switch {
		case child.Type != nethtml.ElementNode:
		case hasClass(child, "wp-block-media-text__media"):
			media = child
		case hasClass(child, "wp-block-media-text__content"):
			stackColumns(child)
			markNode(child)
			for inner := child.FirstChild; inner != nil; inner = inner.NextSibling {
				content = append(content, inner)
			}
		}
	}
	if media == nil || !fitsColumn([]*nethtml.Node{media}) || !fitsColumn(content) {
		return
	}
	contents := [][]*nethtml.Node{{media}, content}
	if hasClass(block, "has-media-on-the-right") {
		contents[0], contents[1] = contents[1], contents[0]
	}
	valign := "center"
	switch {
	case hasClass(block, "is-vertically-aligned-top"):
		valign = ""
	case hasClass(block, "is-vertically-aligned-bottom"):
		valign = "bottom"
	}
	replaceWithColumns(block, "", valign, contents)
}

var backgroundImage = regexp.MustCompile(`background-image:\s*url\(\s*['"]?([^'")\s]+)`)

// markCover turns a wp:cover block with a background image into markers
// around its content, with the image first. toMarkdown turns them into a
// pr-hero. It reports false and leaves the block as it is when there's no
// image or no content.
func markCover(cover *nethtml.Node) bool {
	var inner *nethtml.Node
	for _, div := range allElements(cover, "div") {
		if hasClass(div, "wp-block-cover__inner-container") {
			inner = div
			break
		}
	}
	src, alt := coverImage(cover)
	if inner == nil || src == "" {
		return false
	}
	align := alignOf(cover)
	for child := inner.FirstChild; child != nil && align == ""; child = child.NextSibling {
		if child.Type == nethtml.ElementNode && isTextBlock(child) {
			align = alignOf(child)
		}
	}
	attributes := ""
	if alt = strings.ReplaceAll(strings.ReplaceAll(alt, `"`, ""), `\`, ""); alt != "" && !strings.Contains(alt, "{{") {
		attributes += fmt.Sprintf(` alt="%s"`, alt)
	}
	if link := firstButton(inner); link != nil {
		text := plainText(renderChildren(link))
		if buttonShortcode(attr(link, "href"), text, "") != "" {
			attributes += fmt.Sprintf(` button="%s" link="%s"`, text, attr(link, "href"))
			removeButton(link)
		}
	}
	if align != "" {
		attributes += fmt.Sprintf(` align="%s"`, align)
	}

	picture := &nethtml.Node{Type: nethtml.ElementNode, Data: "p"}
	picture.AppendChild(&nethtml.Node{Type: nethtml.ElementNode, Data: "img", Attr: []nethtml.Attribute{{Key: "src", Val: src}}})
	parent := cover.Parent
	parent.InsertBefore(markerParagraph("PRHEROSTART"+hex.EncodeToString([]byte(attributes))), cover)
	parent.InsertBefore(picture, cover)
	for inner.FirstChild != nil {
		node := inner.FirstChild
		inner.RemoveChild(node)
		parent.InsertBefore(node, cover)
	}
	parent.InsertBefore(markerParagraph("PRHEROEND"), cover)
	parent.RemoveChild(cover)
	return true
}

// coverImage is the address and alt text of the cover's background image,
// from an img or from a background-image style.
func coverImage(cover *nethtml.Node) (string, string) {
	for _, img := range allElements(cover, "img") {
		if hasClass(img, "wp-block-cover__image-background") && attr(img, "src") != "" {
			return attr(img, "src"), attr(img, "alt")
		}
	}
	for _, el := range append([]*nethtml.Node{cover}, allElements(cover, "div")...) {
		if found := backgroundImage.FindStringSubmatch(attr(el, "style")); found != nil && (el == cover || hasClass(el, "wp-block-cover__image-background")) {
			return found[1], attr(el, "aria-label")
		}
	}
	return "", ""
}

func firstButton(inner *nethtml.Node) *nethtml.Node {
	for _, link := range allElements(inner, "a") {
		if hasClass(link, "wp-block-button__link") {
			return link
		}
	}
	return nil
}

// removeButton takes a button out of its row, and the row when it's empty.
func removeButton(link *nethtml.Node) {
	button := link
	if hasClass(link.Parent, "wp-block-button") {
		button = link.Parent
	}
	row := button.Parent
	row.RemoveChild(button)
	if hasClass(row, "wp-block-buttons") && !hasElement(row) {
		row.Parent.RemoveChild(row)
	}
}

func hasElement(el *nethtml.Node) bool {
	for child := el.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.ElementNode {
			return true
		}
	}
	return false
}

func replaceWithColumns(block *nethtml.Node, layout, valign string, contents [][]*nethtml.Node) {
	attributes := ""
	if layout != "" {
		attributes += fmt.Sprintf(` layout="%s"`, layout)
	}
	if valign != "" {
		attributes += fmt.Sprintf(` valign="%s"`, valign)
	}
	parent := block.Parent
	parent.InsertBefore(markerParagraph("PRCOLUMNSSTART"+hex.EncodeToString([]byte(attributes))), block)
	for index, content := range contents {
		if index > 0 {
			parent.InsertBefore(markerParagraph("PRCOLUMNNEXT"), block)
		}
		for _, node := range content {
			node.Parent.RemoveChild(node)
			parent.InsertBefore(node, block)
		}
	}
	parent.InsertBefore(markerParagraph("PRCOLUMNSEND"), block)
	parent.RemoveChild(block)
}

// columnsLayout reads the layout from the columns' widths. A column without
// a width takes an equal part of what the others leave.
func columnsLayout(columns []*nethtml.Node) (string, bool) {
	if len(columns) != 2 && len(columns) != 3 {
		return "", false
	}
	widths := make([]float64, len(columns))
	taken, unset := 0.0, 0
	for index, column := range columns {
		if found := flexBasis.FindStringSubmatch(attr(column, "style")); found != nil {
			widths[index], _ = strconv.ParseFloat(found[1], 64)
			taken += widths[index]
		} else {
			unset++
		}
	}
	if unset > 0 {
		if taken >= 100 {
			return "", false
		}
		for index := range widths {
			if widths[index] == 0 {
				widths[index] = (100 - taken) / float64(unset)
			}
		}
	}
	total := 0.0
	for _, width := range widths {
		total += width
	}
	near := func(width, share float64) bool { return math.Abs(width/total-share) <= 0.05 }
	switch {
	case len(columns) == 3 && near(widths[0], 1.0/3) && near(widths[1], 1.0/3) && near(widths[2], 1.0/3):
		return "1-1-1", true
	case len(columns) == 2 && near(widths[0], 0.5):
		return "", true
	case len(columns) == 2 && near(widths[0], 2.0/3):
		return "2-1", true
	case len(columns) == 2 && near(widths[0], 1.0/3):
		return "1-2", true
	}
	return "", false
}

// verticalAlign is the alignment all the blocks share, "center" or "bottom".
func verticalAlign(blocks ...*nethtml.Node) string {
	for _, align := range []string{"center", "bottom"} {
		all := len(blocks) > 0
		for _, block := range blocks {
			if !hasClass(block, "is-vertically-aligned-"+align) && !hasClass(block, "are-vertically-aligned-"+align) {
				all = false
			}
		}
		if all {
			return align
		}
	}
	return ""
}

// fitsColumn tells whether the editor's columns can hold these nodes: text
// blocks, lists, tables, images, videos, and the markers of buttons and aligned
// blocks. Columns already marked inside can't nest in the editor.
func fitsColumn(nodes []*nethtml.Node) bool {
	for _, node := range nodes {
		switch {
		case node.Type == nethtml.TextNode && strings.TrimSpace(node.Data) == "":
		case node.Type != nethtml.ElementNode:
			return false
		case node.Data == "p" && node.FirstChild != nil && (strings.HasPrefix(node.FirstChild.Data, "PRCOLUMN") || strings.HasPrefix(node.FirstChild.Data, "PRHERO")):
			return false
		case isColumnMedia(node):
		case slices.Contains([]string{"p", "h1", "h2", "h3", "h4", "ul", "ol", "blockquote", "table", "pre", "figure"}, node.Data):
			if !fitsInside(node) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func fitsInside(el *nethtml.Node) bool {
	for child := el.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != nethtml.ElementNode {
			continue
		}
		switch {
		case isColumnMedia(child):
		case slices.Contains([]string{"div", "section", "aside", "iframe", "script", "style", "audio", "video", "hr", "h5", "h6", "figure", "form", "details"}, child.Data):
			return false
		}
		if !fitsInside(child) {
			return false
		}
	}
	return true
}

// isColumnMedia tells the videos and embeds that come out as pr-video.
func isColumnMedia(el *nethtml.Node) bool {
	switch el.Data {
	case "video":
		src := attr(el, "src")
		for child := el.FirstChild; child != nil && src == ""; child = child.NextSibling {
			if child.Type == nethtml.ElementNode && child.Data == "source" {
				src = attr(child, "src")
			}
		}
		return src != "" && !strings.Contains(src, `"`)
	case "iframe":
		return strings.HasPrefix(embedShortcode(attr(el, "src"), ""), "{{< pr-video")
	}
	return false
}
