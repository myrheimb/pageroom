package wordpress

import (
	"encoding/hex"
	"fmt"
	"strings"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// markLayout puts text markers around the aligned blocks and in place of the
// buttons in prepared HTML. toMarkdown turns the markers into pr-align and
// pr-button shortcodes once the blocks are Markdown.
func markLayout(content string) string {
	if !strings.Contains(content, "align") && !strings.Contains(content, "wp-block-button") {
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
