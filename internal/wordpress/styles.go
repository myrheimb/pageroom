package wordpress

import (
	"cmp"
	"html"
	"math"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/myrheimb/pageroom/internal/appearance"
)

var (
	styleBlock    = regexp.MustCompile(`(?s)<style([^>]*)>(.*?)</style>`)
	cssComment    = regexp.MustCompile(`(?s)/\*.*?\*/`)
	variable      = regexp.MustCompile(`^var\(\s*(--[\w-]+)\s*(?:,\s*(.*))?\)$`)
	hexColor      = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	shortHexColor = regexp.MustCompile(`^#([0-9a-f])([0-9a-f])([0-9a-f])$`)
	pixels        = regexp.MustCompile(`^(\d+)px$`)
	coreStylesId  = regexp.MustCompile(`id=['"](?:global|classic-theme)-styles-inline-css['"]`)
	linkTag       = regexp.MustCompile(`(?i)<link\b[^>]*>`)
	stylesheetRel = regexp.MustCompile(`(?i)\brel=['"]?[^'">]*\bstylesheet\b`)
	hrefAttribute = regexp.MustCompile(`(?i)\bhref=(?:"([^"]*)"|'([^']*)')`)
	imgTag        = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	customLogo    = regexp.MustCompile(`(?i)\bclass=['"][^'"]*\bcustom-logo\b`)
	siteLogoBlock = regexp.MustCompile(`(?i)class=['"][^'"]*\bwp-block-site-logo\b`)
	srcAttribute  = regexp.MustCompile(`(?i)\bsrc=(?:"([^"]*)"|'([^']*)')`)
	iconOrFontCss = regexp.MustCompile(`(?i)font|icon|dashicons`)
	coverBlock    = regexp.MustCompile(`(?i)\bclass=['"][^'"]*\bwp-block-cover\b`)
)

// globalSelectors are the ones whose custom properties apply to the whole
// page, as opposed to a single component.
var globalSelectors = []string{":root", "html", "body"}

// coreButtonGray is WordPress's own button color, which says nothing about a
// theme's design.
const (
	coreButtonGray   = "#32373c"
	coreButtonRadius = 9999
)

// styleRoles maps the selectors that decide the look of a site to the part
// of the design they stand for. Anything else is ignored.
var styleRoles = map[string]string{
	"html":                              "root",
	"body":                              "body",
	"h1":                                "heading",
	"a":                                 "link",
	"a:where(:not(.wp-element-button))": "link",
	".wp-element-button":                "button",
	".wp-block-button__link":            "button",
	"button":                            "button",
	".button":                           "button",
	".ast-separate-container":           "page",
}

// StylesheetURLs lists the stylesheets a homepage links from its own host,
// in document order, leaving out icon and font stylesheets.
func StylesheetURLs(page []byte, base *url.URL) []string {
	var urls []string
	for _, tag := range linkTag.FindAll(page, -1) {
		href := hrefAttribute.FindSubmatch(tag)
		if href == nil || !stylesheetRel.Match(tag) {
			continue
		}
		link, err := base.Parse(html.UnescapeString(string(href[1]) + string(href[2])))
		if err != nil || link.Host != base.Host || iconOrFontCss.MatchString(link.Path) {
			continue
		}
		link.Fragment = ""
		urls = append(urls, link.String())
	}
	return urls
}

// googleFontsHost serves the CSS of Google Fonts, and googleFontFilesHost
// the font files that CSS points to.
const (
	googleFontsHost     = "fonts.googleapis.com"
	googleFontFilesHost = "fonts.gstatic.com"
)

// GoogleFontsURLs lists the Google Fonts stylesheets a homepage links, in
// document order. Other font services are skipped because their licences
// don't allow copying.
func GoogleFontsURLs(page []byte, base *url.URL) []string {
	var urls []string
	for _, tag := range linkTag.FindAll(page, -1) {
		href := hrefAttribute.FindSubmatch(tag)
		if href == nil || !stylesheetRel.Match(tag) {
			continue
		}
		link, err := base.Parse(html.UnescapeString(string(href[1]) + string(href[2])))
		if err != nil || link.Host != googleFontsHost || link.Scheme != "https" {
			continue
		}
		link.Fragment = ""
		urls = append(urls, link.String())
	}
	return urls
}

// LogoURL finds the logo on a homepage: the custom logo of classic themes,
// else the first image in a Site Logo block. It returns "" if there is none
// or it's not on the site's own host.
func LogoURL(page []byte, base *url.URL) string {
	var logo []byte
	for _, tag := range imgTag.FindAll(page, -1) {
		if customLogo.Match(tag) {
			logo = tag
			break
		}
	}
	if logo == nil {
		if block := siteLogoBlock.FindIndex(page); block != nil {
			logo = imgTag.Find(page[block[1]:])
		}
	}
	src := srcAttribute.FindSubmatch(logo)
	if src == nil {
		return ""
	}
	link, err := base.Parse(html.UnescapeString(string(src[1]) + string(src[2])))
	if err != nil || link.Host != base.Host || (link.Scheme != "http" && link.Scheme != "https") {
		return ""
	}
	return link.String()
}

// headerSelectors are the ones classic themes use for the site header, where
// a photo set as its background is the old site's header photo.
var headerSelectors = []string{".custom-header", ".header", ".site-header", "#header", "#masthead", "header"}

// HeaderPhoto finds the photo behind a homepage's header: the last header
// rule with a background image in the page's own styles. It returns "" if
// there is no photo or it's not on the site's own host. The photo has no alt
// text, since the header already shows the site name.
func HeaderPhoto(page []byte, base *url.URL) (photoURL string) {
	for _, block := range styleBlock.FindAllSubmatch(page, -1) {
		for _, rule := range parseRules(cssComment.ReplaceAllString(string(block[2]), "")) {
			if !slices.ContainsFunc(rule.selectors, func(selector string) bool { return slices.Contains(headerSelectors, selector) }) {
				continue
			}
			for _, declaration := range rule.declarations {
				if declaration.name != "background-image" && declaration.name != "background" {
					continue
				}
				source := fontSourceUrl.FindStringSubmatch(declaration.value)
				if source == nil {
					continue
				}
				link, err := base.Parse(html.UnescapeString(source[1]))
				if err != nil || link.Host != base.Host || (link.Scheme != "http" && link.Scheme != "https") {
					continue
				}
				link.Fragment = ""
				photoURL = link.String()
			}
		}
	}
	return photoURL
}

// fontFamilies are the first family names of the body and heading fonts, as
// the old site wrote them.
type fontFamilies struct{ body, heading string }

const (
	themeStudio = "pageroom-studio"
	themePaper  = "pageroom-paper"
	themeSoft   = "pageroom-soft"
	themeTinted = "pageroom-tinted"

	narrowContentWidth = 720
	largeButtonRadius  = 16
)

// pickTheme chooses the starter theme closest to the old site: Studio for a
// cover on the home page, Paper for serif fonts in a narrow column, Soft for
// rounded buttons, else Tinted.
func pickTheme(homePage []byte, style *appearance.Tokens) string {
	if coverBlock.Match(homePage) {
		return themeStudio
	}
	if style == nil {
		return themeTinted
	}
	serif := style.BodyFont == "transitional" || style.HeadingFont == "transitional"
	if serif && style.ContentWidth > 0 && style.ContentWidth <= narrowContentWidth {
		return themePaper
	}
	if style.Radius >= largeButtonRadius {
		return themeSoft
	}
	return themeTinted
}

func readStyles(page []byte, stylesheets ...[]byte) (*appearance.Tokens, fontFamilies) {
	var core, linked, theme []string
	for _, stylesheet := range stylesheets {
		linked = append(linked, cssComment.ReplaceAllString(string(stylesheet), ""))
	}
	for _, block := range styleBlock.FindAllSubmatch(page, -1) {
		css := cssComment.ReplaceAllString(string(block[2]), "")
		if coreStylesId.Match(block[1]) {
			core = append(core, css)
		} else {
			theme = append(theme, css)
		}
	}

	variables := map[string]string{}
	roles := map[string]map[string]string{}
	for _, css := range slices.Concat(core, linked, theme) {
		for _, rule := range parseRules(css) {
			if slices.ContainsFunc(rule.selectors, func(selector string) bool { return slices.Contains(globalSelectors, selector) }) {
				for _, declaration := range rule.declarations {
					if strings.HasPrefix(declaration.name, "--") {
						variables[declaration.name] = declaration.value
					}
				}
			}
			for _, selector := range rule.selectors {
				role, ok := styleRoles[selector]
				if !ok {
					continue
				}
				if roles[role] == nil {
					roles[role] = map[string]string{}
				}
				for _, declaration := range rule.declarations {
					roles[role][declaration.name] = declaration.value
				}
			}
		}
	}
	resolve := func(value string) string {
		for range 10 {
			reference := variable.FindStringSubmatch(value)
			if reference == nil {
				break
			}
			value = cmp.Or(variables[reference[1]], strings.TrimSpace(reference[2]))
		}
		return value
	}
	property := func(role, name string) string { return resolve(roles[role][name]) }
	background := func(role string) string {
		return cmp.Or(color(property(role, "background-color")), color(property(role, "background")))
	}
	buttonColor := background("button")
	if buttonColor == coreButtonGray {
		buttonColor = ""
	}

	radiusValue := strings.TrimSpace(property("button", "border-radius"))
	if radiusValue == "0" {
		radiusValue = "0px"
	}
	radius := cssPixels(radiusValue, -1)
	if radius == coreButtonRadius {
		radius = -1
	}
	radius = min(radius, appearance.MaxRadius)

	tokens := appearance.Tokens{
		Background:   cmp.Or(background("body"), background("page")),
		Text:         color(property("body", "color")),
		Heading:      color(property("heading", "color")),
		BodyFont:     fontStack(property("body", "font-family")),
		HeadingFont:  fontStack(property("heading", "font-family")),
		ContentWidth: contentWidth(resolve(variables["--wp--style--global--content-size"])),
		// Links often just follow the text color, and then the buttons carry the accent.
		Primary:  primaryColor(color(property("link", "color")), buttonColor),
		FontSize: fontSize(cmp.Or(property("body", "font-size"), property("root", "font-size")), rootSize(property("root", "font-size"))),
		Radius:   radius,
	}
	if tokens.Validate() != nil {
		tokens.FontSize = 0
	}
	families := fontFamilies{body: firstFamily(property("body", "font-family")), heading: firstFamily(property("heading", "font-family"))}
	if reflect.DeepEqual(tokens, appearance.Tokens{Radius: -1}) {
		return nil, families
	}
	return &tokens, families
}

// primaryColor prefers the link color, unless it is a gray that only follows
// the text and the button has a color of its own.
func primaryColor(link, button string) string {
	if button != "" && len(link) == 7 && link[1:3] == link[3:5] && link[3:5] == link[5:7] {
		return button
	}
	return cmp.Or(link, button)
}

var (
	fontFaceRule      = regexp.MustCompile(`(?is)@font-face\s*\{([^}]*)\}`)
	fontFamilyValue   = regexp.MustCompile(`(?is)(?:^|[;\s])font-family\s*:\s*([^;]+)`)
	fontWeightValue   = regexp.MustCompile(`(?is)(?:^|[;\s])font-weight\s*:\s*([^;]+)`)
	fontStyleValue    = regexp.MustCompile(`(?is)(?:^|[;\s])font-style\s*:\s*([^;]+)`)
	unicodeRangeValue = regexp.MustCompile(`(?is)(?:^|[;\s])unicode-range\s*:\s*([^;]+)`)
	fontSourceUrl     = regexp.MustCompile(`(?i)url\(\s*['"]?([^'")\s]+)['"]?\s*\)`)
)

type FontFace struct {
	Family, URL, Weight, Style, UnicodeRange string
}

// fontFormats are the font file types, best first.
var fontFormats = []string{".woff2", ".woff", ".ttf", ".otf"}

// FontFaces lists the @font-face rules of a stylesheet, each with its best
// web address (relative ones resolved against base). Rules without a usable
// address are skipped.
func FontFaces(css string, base *url.URL) []FontFace {
	var faces []FontFace
	css = cssComment.ReplaceAllString(css, "")
	for _, rule := range fontFaceRule.FindAllStringSubmatch(css, -1) {
		declaration := func(pattern *regexp.Regexp) string {
			match := pattern.FindStringSubmatch(rule[1])
			if match == nil {
				return ""
			}
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(match[1]), "!important"))
		}
		face := FontFace{
			Family:       firstFamily(declaration(fontFamilyValue)),
			Weight:       cmp.Or(declaration(fontWeightValue), "400"),
			Style:        cmp.Or(declaration(fontStyleValue), "normal"),
			UnicodeRange: declaration(unicodeRangeValue),
		}
		switch face.Weight {
		case "normal":
			face.Weight = "400"
		case "bold":
			face.Weight = "700"
		}

		var best *url.URL
		bestRank := len(fontFormats)
		for _, source := range fontSourceUrl.FindAllStringSubmatch(rule[1], -1) {
			link, err := base.Parse(html.UnescapeString(source[1]))
			if err != nil || (link.Scheme != "http" && link.Scheme != "https") {
				continue
			}
			if rank := slices.Index(fontFormats, strings.ToLower(path.Ext(link.Path))); rank >= 0 && rank < bestRank {
				best, bestRank = link, rank
			}
		}
		if face.Family == "" || best == nil {
			continue
		}
		best.Fragment = ""
		face.URL = best.String()
		faces = append(faces, face)
	}
	return faces
}

func firstFamily(value string) string {
	first, _, _ := strings.Cut(value, ",")
	return strings.Trim(strings.TrimSpace(first), `"'`)
}

type cssDeclaration struct{ name, value string }

type cssRule struct {
	selectors    []string
	declarations []cssDeclaration
}

// parseRules reads the plain style rules of a stylesheet, in order. At-rules
// such as media queries are skipped, since they only apply to some screens.
func parseRules(css string) []cssRule {
	var rules []cssRule
	for {
		open := strings.Index(css, "{")
		if open < 0 {
			return rules
		}
		prelude := css[:open]
		if semicolon := strings.LastIndex(prelude, ";"); semicolon >= 0 {
			prelude = prelude[semicolon+1:]
		}
		prelude = strings.TrimSpace(prelude)

		depth, end := 0, len(css)
		for index := open; index < len(css); index++ {
			if css[index] == '{' {
				depth++
			} else if css[index] == '}' {
				depth--
				if depth == 0 {
					end = index
					break
				}
			}
		}
		body := css[open+1 : min(end, len(css))]
		css = css[min(end+1, len(css)):]
		if strings.HasPrefix(prelude, "@") {
			continue
		}

		rule := cssRule{}
		for _, selector := range splitSelectors(prelude) {
			// WordPress 6.6 and later wrap most selectors in ":root :where(…)".
			if inner, wrapped := strings.CutPrefix(selector, ":root :where("); wrapped {
				rule.selectors = append(rule.selectors, splitSelectors(strings.TrimSuffix(inner, ")"))...)
				continue
			}
			rule.selectors = append(rule.selectors, selector)
		}
		for _, declaration := range strings.Split(body, ";") {
			name, value, ok := strings.Cut(declaration, ":")
			if !ok {
				continue
			}
			value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "!important"))
			rule.declarations = append(rule.declarations, cssDeclaration{strings.TrimSpace(name), value})
		}
		rules = append(rules, rule)
	}
}

// splitSelectors splits a selector list at the commas outside parentheses.
func splitSelectors(list string) []string {
	var selectors []string
	depth, start := 0, 0
	for index, char := range list {
		switch char {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				selectors = append(selectors, strings.TrimSpace(list[start:index]))
				start = index + 1
			}
		}
	}
	return append(selectors, strings.TrimSpace(list[start:]))
}

func color(value string) string {
	value = shortHexColor.ReplaceAllString(strings.ToLower(value), "#$1$1$2$2$3$3")
	if !hexColor.MatchString(value) {
		return ""
	}
	return value
}

// fontStack picks the closest of Pageroom's fonts, since the site's own
// fonts would have to be loaded from WordPress or a font service.
func fontStack(family string) string {
	family = strings.ToLower(family)
	switch {
	case strings.Contains(family, "mono"):
		return "mono"
	case strings.Contains(family, "slab"):
		return "slab"
	case strings.Contains(family, "sans"), strings.Contains(family, "system-ui"), strings.Contains(family, "-apple-system"):
		return "system"
	case strings.Contains(family, "serif"):
		return "transitional"
	}
	return ""
}

// rootSize reads the size of 1rem from a percentage or px html font size,
// defaulting to the browser's 16px.
func rootSize(value string) float64 {
	value = strings.TrimSpace(value)
	if percent, ok := strings.CutSuffix(value, "%"); ok {
		if size, err := strconv.ParseFloat(percent, 64); err == nil && size > 0 {
			return 16 * size / 100
		}
	}
	if match := pixels.FindStringSubmatch(value); match != nil {
		if size, _ := strconv.Atoi(match[1]); size > 0 {
			return float64(size)
		}
	}
	return 16
}

// fontSize reads a size in px, or in rem at the given size of 1rem.
func fontSize(value string, remSize float64) int {
	value = strings.TrimSpace(value)
	if rem, ok := strings.CutSuffix(value, "rem"); ok {
		size, err := strconv.ParseFloat(rem, 64)
		if err != nil {
			return 0
		}
		return int(math.Round(size * remSize))
	}
	return cssPixels(value, 0)
}

func cssPixels(value string, fallback int) int {
	match := pixels.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return fallback
	}
	size, _ := strconv.Atoi(match[1])
	return size
}

func contentWidth(value string) int {
	match := pixels.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return 0
	}
	width, _ := strconv.Atoi(match[1])
	if (appearance.Tokens{ContentWidth: width}).Validate() != nil {
		return 0
	}
	return width
}
