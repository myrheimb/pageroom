package frontmatter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// tomlEditor edits TOML front matter as lines, so comments, ordering and
// formatting of untouched keys survive every save.
type tomlEditor struct {
	lines []string
}

type tomlItemKind int

const (
	tomlAssign tomlItemKind = iota // key = value (top level)
	tomlHeader                     // [table] or [[array]]
)

type tomlItem struct {
	kind       tomlItemKind
	key        string // first key segment
	start, end int    // line range [start, end)
}

func newTOMLEditor(src []byte) (*tomlEditor, error) {
	if len(bytes.TrimSpace(src)) > 0 {
		var probe map[string]any
		if err := toml.Unmarshal(src, &probe); err != nil {
			return nil, fmt.Errorf("front matter (TOML) has a syntax error: %w", err)
		}
	}
	s := strings.TrimSuffix(string(src), "\n")
	e := &tomlEditor{}
	if s != "" {
		e.lines = strings.Split(s, "\n")
	}
	return e, nil
}

func (e *tomlEditor) values() map[string]any {
	m := map[string]any{}
	_ = toml.Unmarshal([]byte(strings.Join(e.lines, "\n")), &m)
	return m
}

// scan walks the lines and returns top-level assignments and table headers.
// Inside a table, assignments belong to the table and are not reported.
func (e *tomlEditor) scan() []tomlItem {
	var items []tomlItem
	inTable := false
	for i := 0; i < len(e.lines); {
		t := strings.TrimSpace(e.lines[i])
		switch {
		case t == "" || strings.HasPrefix(t, "#"):
			i++
		case strings.HasPrefix(t, "["):
			inTable = true
			items = append(items, tomlItem{kind: tomlHeader, key: headerKey(t), start: i, end: i + 1})
			i++
		default:
			end := e.valueEnd(i)
			if !inTable {
				items = append(items, tomlItem{kind: tomlAssign, key: assignKey(t), start: i, end: end})
			}
			i = end
		}
	}
	return items
}

// valueEnd finds where an assignment starting at line i ends, by growing the
// candidate until it parses (handles multi-line arrays and strings).
func (e *tomlEditor) valueEnd(i int) int {
	for j := i + 1; j <= len(e.lines) && j <= i+500; j++ {
		var probe map[string]any
		if toml.Unmarshal([]byte(strings.Join(e.lines[i:j], "\n")), &probe) == nil {
			return j
		}
	}
	return i + 1
}

func assignKey(line string) string {
	k, _, _ := strings.Cut(line, "=")
	return firstSegment(strings.TrimSpace(k))
}

func headerKey(line string) string {
	t := strings.TrimLeft(line, "[")
	if i := strings.Index(t, "]"); i >= 0 {
		t = t[:i]
	}
	return firstSegment(strings.TrimSpace(t))
}

func firstSegment(k string) string {
	if strings.HasPrefix(k, `"`) || strings.HasPrefix(k, `'`) {
		q := k[:1]
		if end := strings.Index(k[1:], q); end >= 0 {
			return k[1 : end+1]
		}
	}
	seg, _, _ := strings.Cut(k, ".")
	return strings.TrimSpace(seg)
}

func (e *tomlEditor) Keys() []string {
	seen := map[string]bool{}
	var keys []string
	for _, it := range e.scan() {
		if !seen[it.key] {
			seen[it.key] = true
			keys = append(keys, it.key)
		}
	}
	return keys
}

func (e *tomlEditor) Get(key string) (any, bool) {
	v, ok := e.values()[key]
	if !ok {
		return nil, false
	}
	return normalizeTOML(v), true
}

// normalizeTOML turns TOML-specific date types into time-like strings the UI
// understands, recursively.
func normalizeTOML(v any) any {
	switch t := v.(type) {
	case toml.LocalDate:
		return t.String()
	case toml.LocalDateTime:
		return t.String()
	case toml.LocalTime:
		return t.String()
	case map[string]any:
		for k, x := range t {
			t[k] = normalizeTOML(x)
		}
	case []any:
		for i, x := range t {
			t[i] = normalizeTOML(x)
		}
	}
	return v
}

func (e *tomlEditor) Set(key string, value any) error {
	enc, err := toml.Marshal(map[string]any{key: value})
	if err != nil {
		return err
	}
	newLines := strings.Split(strings.TrimRight(string(enc), "\n"), "\n")
	isTable := len(newLines) > 0 && strings.HasPrefix(strings.TrimSpace(newLines[0]), "[")

	items := e.scan()
	// Replace an existing inline assignment in place when possible.
	if !isTable {
		for _, it := range items {
			if it.kind == tomlAssign && it.key == key && !e.hasTableFor(key, items) {
				e.splice(it.start, it.end, newLines)
				return nil
			}
		}
	}
	e.Delete(key)
	if isTable {
		if len(e.lines) > 0 && strings.TrimSpace(e.lines[len(e.lines)-1]) != "" {
			e.lines = append(e.lines, "")
		}
		e.lines = append(e.lines, newLines...)
		return nil
	}
	// New inline keys go at the end of the top-level section.
	at := len(e.lines)
	for _, it := range e.scan() {
		if it.kind == tomlHeader {
			at = it.start
			for at > 0 && strings.TrimSpace(e.lines[at-1]) == "" {
				at--
			}
			break
		}
	}
	e.splice(at, at, newLines)
	return nil
}

func (e *tomlEditor) hasTableFor(key string, items []tomlItem) bool {
	for _, it := range items {
		if it.kind == tomlHeader && it.key == key {
			return true
		}
	}
	return false
}

func (e *tomlEditor) Delete(key string) {
	for {
		items := e.scan()
		removed := false
		for idx := len(items) - 1; idx >= 0; idx-- {
			it := items[idx]
			if it.key != key {
				continue
			}
			end := it.end
			if it.kind == tomlHeader {
				// a table runs until the next header
				end = len(e.lines)
				if idx+1 < len(items) {
					for _, next := range items[idx+1:] {
						if next.kind == tomlHeader {
							end = next.start
							break
						}
					}
				}
			}
			e.splice(it.start, end, nil)
			removed = true
			break
		}
		if !removed {
			return
		}
	}
}

func (e *tomlEditor) splice(start, end int, repl []string) {
	out := make([]string, 0, len(e.lines)-(end-start)+len(repl))
	out = append(out, e.lines[:start]...)
	out = append(out, repl...)
	out = append(out, e.lines[end:]...)
	e.lines = out
}

func (e *tomlEditor) Encode() ([]byte, error) {
	// trim trailing blank lines left over from deletions
	for len(e.lines) > 0 && strings.TrimSpace(e.lines[len(e.lines)-1]) == "" {
		e.lines = e.lines[:len(e.lines)-1]
	}
	if len(e.lines) == 0 {
		return nil, nil
	}
	s := strings.Join(e.lines, "\n") + "\n"
	var probe map[string]any
	if err := toml.Unmarshal([]byte(s), &probe); err != nil {
		return nil, fmt.Errorf("internal error: edited TOML is invalid: %w", err)
	}
	return []byte(s), nil
}
