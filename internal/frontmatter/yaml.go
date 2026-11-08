package frontmatter

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

type yamlEditor struct {
	root *yaml.Node // document node
	m    *yaml.Node // top-level mapping
}

func newYAMLEditor(src []byte) (*yamlEditor, error) {
	e := &yamlEditor{root: &yaml.Node{Kind: yaml.DocumentNode}}
	if len(bytes.TrimSpace(src)) > 0 {
		if err := yaml.Unmarshal(src, e.root); err != nil {
			return nil, fmt.Errorf("front matter (YAML) has a syntax error: %w", err)
		}
	}
	if len(e.root.Content) == 0 {
		e.root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	e.m = e.root.Content[0]
	if e.m.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("front matter (YAML) must be a set of key: value pairs")
	}
	return e, nil
}

func (e *yamlEditor) Keys() []string {
	keys := make([]string, 0, len(e.m.Content)/2)
	for i := 0; i+1 < len(e.m.Content); i += 2 {
		keys = append(keys, e.m.Content[i].Value)
	}
	return keys
}

func (e *yamlEditor) find(key string) int {
	for i := 0; i+1 < len(e.m.Content); i += 2 {
		if e.m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func (e *yamlEditor) Get(key string) (any, bool) {
	i := e.find(key)
	if i < 0 {
		return nil, false
	}
	var v any
	if err := e.m.Content[i+1].Decode(&v); err != nil {
		return nil, false
	}
	return normalize(v), true
}

func (e *yamlEditor) Set(key string, value any) error {
	var n yaml.Node
	if err := n.Encode(value); err != nil {
		return err
	}
	if i := e.find(key); i >= 0 {
		old := e.m.Content[i+1]
		// keep the comments the user wrote next to the value
		n.LineComment, n.HeadComment, n.FootComment = old.LineComment, old.HeadComment, old.FootComment
		if old.Kind == yaml.SequenceNode && n.Kind == yaml.SequenceNode {
			n.Style = old.Style // keep [a, b] vs block style
		}
		e.m.Content[i+1] = &n
		return nil
	}
	e.m.Content = append(e.m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &n)
	return nil
}

func (e *yamlEditor) Delete(key string) {
	if i := e.find(key); i >= 0 {
		e.m.Content = append(e.m.Content[:i], e.m.Content[i+2:]...)
	}
}

func (e *yamlEditor) Encode() ([]byte, error) {
	if len(e.m.Content) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(e.root); err != nil {
		return nil, err
	}
	enc.Close()
	return buf.Bytes(), nil
}

// normalize converts decoded values into JSON-friendly shapes
// (map[string]any instead of map[any]any) so the UI receives clean data.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = normalize(x)
		}
		return t
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[fmt.Sprint(k)] = normalize(x)
		}
		return out
	case []any:
		for i, x := range t {
			t[i] = normalize(x)
		}
		return t
	default:
		return v
	}
}

func equalValues(a, b any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(ja, jb)
}
