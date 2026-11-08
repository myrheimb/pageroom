// Package frontmatter reads and edits Hugo front matter (YAML, TOML or JSON)
// without disturbing what the user didn't change. Untouched documents
// round-trip byte-for-byte, YAML keeps comments and key order, TOML is edited
// line by line, and JSON keeps key order.
package frontmatter

import (
	"bytes"
	"errors"
	"fmt"
)

type Format string

const (
	None Format = ""
	YAML Format = "yaml"
	TOML Format = "toml"
	JSON Format = "json"
)

type Document struct {
	Format Format
	Body   []byte

	crlf    bool
	bom     bool
	rawFM   []byte // front matter between delimiters, LF-normalised
	dirty   bool
	editor  editor
	trailer []byte // bytes between closing delimiter and body ("\n" usually)
	whole   bool   // the entire file is data (a site config file), no delimiters
}

type editor interface {
	Keys() []string
	Get(key string) (any, bool)
	Set(key string, value any) error
	Delete(key string)
	Encode() ([]byte, error)
}

var bomBytes = []byte{0xEF, 0xBB, 0xBF}

// Parse splits data into front matter and body. Files without front matter
// are valid and get Format None until a field is set.
func Parse(data []byte) (*Document, error) {
	d := &Document{}
	if bytes.HasPrefix(data, bomBytes) {
		d.bom = true
		data = data[len(bomBytes):]
	}
	d.crlf = bytes.Contains(data, []byte("\r\n"))
	norm := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	switch {
	case bytes.HasPrefix(norm, []byte("---\n")) || bytes.Equal(norm, []byte("---")):
		return d, d.split(norm, "---", YAML)
	case bytes.HasPrefix(norm, []byte("+++\n")):
		return d, d.split(norm, "+++", TOML)
	case bytes.HasPrefix(norm, []byte("{")):
		end, err := jsonObjectEnd(norm)
		if err != nil {
			return nil, err
		}
		d.Format = JSON
		d.rawFM = norm[:end]
		rest := norm[end:]
		d.trailer, d.Body = splitTrailer(rest)
		ed, err := newJSONEditor(d.rawFM)
		if err != nil {
			return nil, err
		}
		d.editor = ed
		return d, nil
	default:
		d.Body = norm
		return d, nil
	}
}

func (d *Document) split(norm []byte, delim string, f Format) error {
	rest := norm[len(delim):]
	rest = bytes.TrimPrefix(rest, []byte("\n"))
	closing := []byte("\n" + delim)
	var fm []byte
	if bytes.HasPrefix(rest, []byte(delim)) { // empty front matter
		fm, rest = nil, rest[len(delim):]
	} else {
		i := indexLine(rest, delim)
		if i < 0 {
			return fmt.Errorf("front matter starts with %q but is never closed", delim)
		}
		fm, rest = rest[:i+1], rest[i+len(closing):]
	}
	d.Format = f
	d.rawFM = fm
	d.trailer, d.Body = splitTrailer(rest)
	var err error
	switch f {
	case YAML:
		d.editor, err = newYAMLEditor(fm)
	case TOML:
		d.editor, err = newTOMLEditor(fm)
	}
	return err
}

// indexLine finds "\n"+delim followed by end of line or EOF. It returns the index of the "\n".
func indexLine(b []byte, delim string) int {
	off := 0
	needle := []byte("\n" + delim)
	for {
		i := bytes.Index(b[off:], needle)
		if i < 0 {
			return -1
		}
		j := off + i + len(needle)
		if j == len(b) || b[j] == '\n' || b[j] == ' ' || b[j] == '\t' {
			return off + i
		}
		off = j
	}
}

func splitTrailer(rest []byte) (trailer, body []byte) {
	n := 0
	for n < len(rest) && (rest[n] == ' ' || rest[n] == '\t') {
		n++
	}
	if n < len(rest) && rest[n] == '\n' {
		n++
	}
	return rest[:n], rest[n:]
}

// ParseData parses a whole data or config file (hugo.toml, menus.yaml...)
// with the same lossless editors used for front matter.
func ParseData(data []byte, f Format) (*Document, error) {
	d := &Document{Format: f, whole: true}
	if bytes.HasPrefix(data, bomBytes) {
		d.bom = true
		data = data[len(bomBytes):]
	}
	d.crlf = bytes.Contains(data, []byte("\r\n"))
	norm := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	d.rawFM = norm
	var err error
	switch f {
	case YAML:
		d.editor, err = newYAMLEditor(norm)
	case TOML:
		d.editor, err = newTOMLEditor(norm)
	case JSON:
		if len(bytes.TrimSpace(norm)) == 0 {
			norm = []byte("{}")
		}
		d.editor, err = newJSONEditor(norm)
	default:
		return nil, fmt.Errorf("unsupported data format %q", f)
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

func FormatFromExt(ext string) Format {
	switch ext {
	case ".yaml", ".yml":
		return YAML
	case ".toml":
		return TOML
	case ".json":
		return JSON
	}
	return None
}

func (d *Document) Keys() []string {
	if d.editor == nil {
		return nil
	}
	return d.editor.Keys()
}

func (d *Document) Get(key string) (any, bool) {
	if d.editor == nil {
		return nil, false
	}
	return d.editor.Get(key)
}

func (d *Document) Fields() map[string]any {
	out := map[string]any{}
	for _, k := range d.Keys() {
		v, _ := d.Get(k)
		out[k] = v
	}
	return out
}

// Set changes or adds a value. If the document has no front matter yet, it
// is created in the given fallback format.
func (d *Document) Set(key string, value any, fallback Format) error {
	if key == "" {
		return errors.New("empty front matter key")
	}
	if d.editor == nil {
		if fallback == None {
			fallback = YAML
		}
		d.Format = fallback
		var err error
		switch fallback {
		case YAML:
			d.editor, err = newYAMLEditor(nil)
		case TOML:
			d.editor, err = newTOMLEditor(nil)
		case JSON:
			d.editor, err = newJSONEditor([]byte("{}"))
		}
		if err != nil {
			return err
		}
		d.trailer = []byte("\n")
	}
	if old, ok := d.editor.Get(key); ok && equalValues(old, value) {
		return nil
	}
	d.dirty = true
	return d.editor.Set(key, value)
}

func (d *Document) Delete(key string) {
	if d.editor == nil {
		return
	}
	if _, ok := d.editor.Get(key); ok {
		d.dirty = true
		d.editor.Delete(key)
	}
}

func (d *Document) SetBody(body []byte) {
	d.Body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
}

// Bytes serialises the document, reproducing the original bytes for every
// part that was not modified.
func (d *Document) Bytes() ([]byte, error) {
	var out bytes.Buffer
	if d.whole {
		fm := d.rawFM
		if d.dirty {
			var err error
			if fm, err = d.editor.Encode(); err != nil {
				return nil, err
			}
		}
		out.Write(fm)
	} else if d.editor != nil {
		fm := d.rawFM
		if d.dirty {
			var err error
			fm, err = d.editor.Encode()
			if err != nil {
				return nil, err
			}
		}
		switch d.Format {
		case YAML:
			out.WriteString("---\n")
			out.Write(fm)
			out.WriteString("---")
		case TOML:
			out.WriteString("+++\n")
			out.Write(fm)
			out.WriteString("+++")
		case JSON:
			out.Write(bytes.TrimRight(fm, "\n"))
		}
		trailer := d.trailer
		if len(trailer) == 0 {
			trailer = []byte("\n")
		}
		out.Write(trailer)
	}
	out.Write(d.Body)
	b := out.Bytes()
	if d.crlf {
		b = bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
	}
	if d.bom {
		b = append(append([]byte{}, bomBytes...), b...)
	}
	return b, nil
}
