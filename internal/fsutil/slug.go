package fsutil

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var transliterations = map[rune]string{
	'æ': "ae", 'ø': "o", 'å': "a", 'ß': "ss", 'œ': "oe", 'þ': "th", 'ð': "d", 'ł': "l",
	'Æ': "ae", 'Ø': "o", 'Å': "a", 'Œ': "oe", 'Þ': "th", 'Ð': "d", 'Ł': "l",
}

// Slugify turns a human title ("Bjørnar's Summer Trip!") into a URL- and
// filesystem-safe slug ("bjornars-summer-trip").
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) { // strip combining accents
			continue
		}
		if t, ok := transliterations[r]; ok {
			b.WriteString(t)
			dash = false
			continue
		}
		switch {
		case r == '\'' || r == '’':
			// drop apostrophes: "it's" -> "its"
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(unicode.ToLower(r))
			dash = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r)) // keep non-latin scripts readable
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 80 {
		out = strings.TrimRight(out[:80], "-")
	}
	if out == "" {
		out = "untitled"
	}
	return out
}
