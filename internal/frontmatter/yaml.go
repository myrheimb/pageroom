package frontmatter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

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
		e.m.Content[i+1] = mergeNode(e.m.Content[i+1], &n)
		return nil
	}
	e.m.Content = append(e.m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &n)
	return nil
}

// mergeNode applies the new value to the existing node, key by key and item by
// item, so unchanged parts keep their comments, order and style. Items match
// by position. It returns the node to keep in the tree.
func mergeNode(old, updated *yaml.Node) *yaml.Node {
	if old.Kind != updated.Kind || old.Anchor != "" || old.Kind == yaml.AliasNode {
		return replaceNode(old, updated)
	}

	switch old.Kind {
	case yaml.ScalarNode:
		var oldValue, newValue any
		if old.Decode(&oldValue) == nil && updated.Decode(&newValue) == nil && equalValues(oldValue, newValue) {
			return old
		}
		if old.Tag == "!!str" && updated.Tag == "!!str" {
			updated.Style = old.Style
		}
		return replaceNode(old, updated)
	case yaml.MappingNode:
		if len(old.Content) == 0 || len(updated.Content) == 0 {
			return replaceNode(old, updated)
		}
		mergeMapping(old, updated)
	case yaml.SequenceNode:
		if len(old.Content) == 0 || len(updated.Content) == 0 {
			return replaceNode(old, updated)
		}
		mergeSequence(old, updated)
	}
	return old
}

func replaceNode(old, updated *yaml.Node) *yaml.Node {
	updated.LineComment, updated.HeadComment, updated.FootComment = old.LineComment, old.HeadComment, old.FootComment
	return updated
}

func mergeMapping(old, updated *yaml.Node) {
	merged := make([]*yaml.Node, 0, len(updated.Content))
	used := make([]bool, len(updated.Content)/2)

	for i := 0; i+1 < len(old.Content); i += 2 {
		for j := 0; j+1 < len(updated.Content); j += 2 {
			if used[j/2] || updated.Content[j].Value != old.Content[i].Value {
				continue
			}
			used[j/2] = true
			merged = append(merged, old.Content[i], mergeNode(old.Content[i+1], updated.Content[j+1]))
			break
		}
	}

	for j := 0; j+1 < len(updated.Content); j += 2 {
		if !used[j/2] {
			merged = append(merged, updated.Content[j], updated.Content[j+1])
		}
	}
	old.Content = merged
}

func mergeSequence(old, updated *yaml.Node) {
	merged := make([]*yaml.Node, 0, len(updated.Content))
	for i, item := range updated.Content {
		if i < len(old.Content) {
			item = mergeNode(old.Content[i], item)
		} else {
			orderLike(item, old.Content[len(old.Content)-1])
		}
		merged = append(merged, item)
	}
	old.Content = merged
}

// orderLike puts the keys of a new item in the order of a sibling item, since
// a new map has no order of its own. Keys the sibling lacks go last.
func orderLike(item, sibling *yaml.Node) {
	if item.Kind != sibling.Kind {
		return
	}

	switch item.Kind {
	case yaml.SequenceNode:
		if len(sibling.Content) == 0 {
			return
		}
		for _, child := range item.Content {
			orderLike(child, sibling.Content[0])
		}
	case yaml.MappingNode:
		ordered := make([]*yaml.Node, 0, len(item.Content))
		used := make([]bool, len(item.Content)/2)

		for i := 0; i+1 < len(sibling.Content); i += 2 {
			for j := 0; j+1 < len(item.Content); j += 2 {
				if used[j/2] || item.Content[j].Value != sibling.Content[i].Value {
					continue
				}
				used[j/2] = true
				orderLike(item.Content[j+1], sibling.Content[i+1])
				ordered = append(ordered, item.Content[j], item.Content[j+1])
				break
			}
		}

		for j := 0; j+1 < len(item.Content); j += 2 {
			if !used[j/2] {
				ordered = append(ordered, item.Content[j], item.Content[j+1])
			}
		}
		item.Content = ordered
	}
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
	if err := enc.Close(); err != nil {
		return nil, err
	}
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

// OrderedMap is a map that YAML writes with Keys first, in that order, and
// any other keys after them in alphabetical order.
type OrderedMap struct {
	Keys   []string
	Values map[string]any
}

func (m OrderedMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Values)
}

func (m OrderedMap) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	written := map[string]bool{}
	add := func(key string) error {
		value, ok := m.Values[key]
		if !ok || written[key] {
			return nil
		}
		written[key] = true
		var valueNode yaml.Node
		if err := valueNode.Encode(value); err != nil {
			return err
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &valueNode)
		return nil
	}

	for _, key := range m.Keys {
		if err := add(key); err != nil {
			return nil, err
		}
	}
	rest := make([]string, 0, len(m.Values))
	for key := range m.Values {
		rest = append(rest, key)
	}
	sort.Strings(rest)
	for _, key := range rest {
		if err := add(key); err != nil {
			return nil, err
		}
	}
	return node, nil
}
