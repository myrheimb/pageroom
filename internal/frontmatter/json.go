package frontmatter

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type jsonEditor struct {
	order  []string
	values map[string]json.RawMessage
}

func jsonObjectEnd(b []byte) (int, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return 0, fmt.Errorf("front matter (JSON) has a syntax error: %w", err)
	}
	return int(dec.InputOffset()), nil
}

func newJSONEditor(src []byte) (*jsonEditor, error) {
	e := &jsonEditor{values: map[string]json.RawMessage{}}
	dec := json.NewDecoder(bytes.NewReader(src))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("front matter (JSON) must be an object")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key := kt.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		if _, dup := e.values[key]; !dup {
			e.order = append(e.order, key)
		}
		e.values[key] = raw
	}
	return e, nil
}

func (e *jsonEditor) Keys() []string { return append([]string(nil), e.order...) }

func (e *jsonEditor) Get(key string) (any, bool) {
	raw, ok := e.values[key]
	if !ok {
		return nil, false
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil, false
	}
	return v, true
}

func (e *jsonEditor) Set(key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, ok := e.values[key]; !ok {
		e.order = append(e.order, key)
	}
	e.values[key] = raw
	return nil
}

func (e *jsonEditor) Delete(key string) {
	if _, ok := e.values[key]; !ok {
		return
	}
	delete(e.values, key)
	for i, k := range e.order {
		if k == key {
			e.order = append(e.order[:i], e.order[i+1:]...)
			break
		}
	}
}

func (e *jsonEditor) Encode() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, k := range e.order {
		kb, _ := json.Marshal(k)
		var vb bytes.Buffer
		if err := json.Indent(&vb, e.values[k], "  ", "  "); err != nil {
			return nil, err
		}
		fmt.Fprintf(&buf, "  %s: %s", kb, vb.Bytes())
		if i < len(e.order)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}
