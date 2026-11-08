// Package wordpress moves a WordPress site into Hugo: posts, pages,
// categories, tags, images, linked files and menus, with old addresses kept
// working through Hugo aliases.
package wordpress

import (
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Export struct {
	Title       string
	Link        string
	Description string
	Authors     map[string]string // login -> display name
	Terms       map[int]Term      // categories and tags by term ID
	Items       []Item
}

type Item struct {
	ID        int
	Title     string
	Link      string
	Date      time.Time
	Creator   string
	Content   string
	Excerpt   string
	Slug      string
	Status    string
	Type      string
	Parent    int
	MenuOrder int
	AttachURL string
	Terms     []Term
	Meta      map[string]string
	Comments  int
}

type Term struct {
	Domain   string // category, post_tag, nav_menu
	Nicename string
	Name     string
}

type xmlText struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

type xmlItem struct {
	Title     string    `xml:"title"`
	Link      string    `xml:"link"`
	PubDate   string    `xml:"pubDate"`
	Creator   string    `xml:"creator"`
	Encoded   []xmlText `xml:"encoded"` // content:encoded and excerpt:encoded
	ID        string    `xml:"post_id"`
	DateGMT   string    `xml:"post_date_gmt"`
	DateLocal string    `xml:"post_date"`
	Name      string    `xml:"post_name"`
	Status    string    `xml:"status"`
	Parent    string    `xml:"post_parent"`
	MenuOrder string    `xml:"menu_order"`
	Type      string    `xml:"post_type"`
	AttachURL string    `xml:"attachment_url"`
	Category  []struct {
		Domain   string `xml:"domain,attr"`
		Nicename string `xml:"nicename,attr"`
		Value    string `xml:",chardata"`
	} `xml:"category"`
	Postmeta []struct {
		Key   string `xml:"meta_key"`
		Value string `xml:"meta_value"`
	} `xml:"postmeta"`
	Comment []struct {
		Approved string `xml:"comment_approved"`
	} `xml:"comment"`
}

type xmlExport struct {
	Channel struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		BaseBlogURL string `xml:"base_blog_url"`
		Authors     []struct {
			Login   string `xml:"author_login"`
			Display string `xml:"author_display_name"`
		} `xml:"author"`
		Categories []struct {
			ID       string `xml:"term_id"`
			Nicename string `xml:"category_nicename"`
			Name     string `xml:"cat_name"`
		} `xml:"category"`
		Tags []struct {
			ID   string `xml:"term_id"`
			Slug string `xml:"tag_slug"`
			Name string `xml:"tag_name"`
		} `xml:"tag"`
		Items []xmlItem `xml:"item"`
	} `xml:"channel"`
}

// Parse reads a WXR export. Go's XML decoder never fetches external
// entities, so a crafted export can't read local files.
func Parse(r io.Reader) (*Export, error) {
	var x xmlExport
	dec := xml.NewDecoder(r)
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	if err := dec.Decode(&x); err != nil {
		return nil, fmt.Errorf("this doesn't look like a WordPress export file: %w", err)
	}
	if len(x.Channel.Items) == 0 && x.Channel.Title == "" {
		return nil, fmt.Errorf("this doesn't look like a WordPress export file")
	}
	e := &Export{Title: strings.TrimSpace(x.Channel.Title), Link: strings.TrimSpace(x.Channel.Link), Description: strings.TrimSpace(x.Channel.Description), Authors: map[string]string{}, Terms: map[int]Term{}}
	if e.Link == "" {
		e.Link = x.Channel.BaseBlogURL
	}
	for _, a := range x.Channel.Authors {
		e.Authors[a.Login] = a.Display
	}
	for _, c := range x.Channel.Categories {
		id, _ := strconv.Atoi(strings.TrimSpace(c.ID))
		e.Terms[id] = Term{Domain: "category", Nicename: c.Nicename, Name: termName(c.Name)}
	}
	for _, t := range x.Channel.Tags {
		id, _ := strconv.Atoi(strings.TrimSpace(t.ID))
		e.Terms[id] = Term{Domain: "post_tag", Nicename: t.Slug, Name: termName(t.Name)}
	}
	for _, xi := range x.Channel.Items {
		it := Item{
			Title: strings.TrimSpace(xi.Title), Link: strings.TrimSpace(xi.Link), Creator: xi.Creator,
			Slug: xi.Name, Status: xi.Status, Type: xi.Type, AttachURL: strings.TrimSpace(xi.AttachURL), Meta: map[string]string{},
		}
		it.ID, _ = strconv.Atoi(strings.TrimSpace(xi.ID))
		it.Parent, _ = strconv.Atoi(strings.TrimSpace(xi.Parent))
		it.MenuOrder, _ = strconv.Atoi(strings.TrimSpace(xi.MenuOrder))
		for _, enc := range xi.Encoded {
			if strings.Contains(enc.XMLName.Space, "excerpt") {
				it.Excerpt = enc.Value
			} else {
				it.Content = enc.Value
			}
		}
		it.Date = parseDate(xi.DateGMT, xi.DateLocal, xi.PubDate)
		for _, c := range xi.Category {
			it.Terms = append(it.Terms, Term{Domain: c.Domain, Nicename: c.Nicename, Name: termName(c.Value)})
		}
		for _, m := range xi.Postmeta {
			it.Meta[m.Key] = m.Value
		}
		for _, c := range xi.Comment {
			if c.Approved == "1" {
				it.Comments++
			}
		}
		e.Items = append(e.Items, it)
	}
	return e, nil
}

// termName undoes the HTML escaping WordPress stores names with, so
// "Coffee &amp; cake" becomes "Coffee & cake".
func termName(name string) string {
	return html.UnescapeString(strings.TrimSpace(name))
}

func parseDate(gmt, local, pub string) time.Time {
	if t, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(gmt)); err == nil && t.Year() > 1970 {
		return t.UTC()
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(local), time.Local); err == nil && t.Year() > 1970 {
		return t
	}
	if t, err := time.Parse(time.RFC1123Z, strings.TrimSpace(pub)); err == nil {
		return t
	}
	return time.Time{}
}

type Summary struct {
	Title     string `json:"title"`
	Link      string `json:"link"`
	Posts     int    `json:"posts"`
	Pages     int    `json:"pages"`
	Drafts    int    `json:"drafts"`
	Images    int    `json:"images"`
	Comments  int    `json:"comments"`
	MenuItems int    `json:"menuItems"`
	// Skipped is content of other kinds, like shop products, that
	// Pageroom doesn't bring over.
	Skipped []SkippedKind `json:"skipped"`
}

type SkippedKind struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// internalTypes are post types WordPress and common plugins use for
// settings, layout and records rather than content.
var internalTypes = map[string]bool{
	"attachment": true, "custom_css": true, "customize_changeset": true, "feedback": true, "nav_menu_item": true,
	"oembed_cache": true, "revision": true, "shop_coupon": true, "shop_order": true, "shop_order_refund": true,
	"user_request": true, "wpcf7_contact_form": true,
}

func (e *Export) Summarize() Summary {
	s := Summary{Title: e.Title, Link: e.Link, Skipped: []SkippedKind{}}
	skipped := map[string]int{}
	for _, it := range e.Items {
		switch it.Type {
		case "post", "page":
			if it.Status == "trash" || it.Status == "auto-draft" {
				continue
			}
			if it.Type == "post" {
				s.Posts++
			} else {
				s.Pages++
			}
			if it.Status != "publish" && it.Status != "future" {
				s.Drafts++
			}
			s.Comments += it.Comments
		case "attachment":
			s.Images++
		case "nav_menu_item":
			s.MenuItems++
		default:
			// Plugins keep their settings as post types too. Those have no page of
			// their own, so WordPress links them as ?post_type=...
			if it.Status == "publish" && !internalTypes[it.Type] && !strings.HasPrefix(it.Type, "wp_") && !strings.Contains(it.Link, "post_type=") {
				skipped[it.Type]++
			}
		}
	}
	for kind, count := range skipped {
		s.Skipped = append(s.Skipped, SkippedKind{Type: kind, Count: count})
	}
	sort.Slice(s.Skipped, func(left, right int) bool {
		if s.Skipped[left].Count != s.Skipped[right].Count {
			return s.Skipped[left].Count > s.Skipped[right].Count
		}
		return s.Skipped[left].Type < s.Skipped[right].Type
	})
	return s
}
