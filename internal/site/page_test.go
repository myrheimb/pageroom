package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const solutionPage = `---
title: Solution
# the steps of the solution
steps:
  - title: One # first
    text: Alpha
    slides:
      - before: a.jpg
        after: b.jpg
      - before: c.jpg
        after: d.jpg
  # the second step
  - title: Two
    text: "Beta"
    slides: []
---
Body
`

func saveSteps(t *testing.T, stepsJson string) string {
	t.Helper()
	s := fixture(t)
	full := filepath.Join(s.Root, "content", "solution.md")
	if err := os.WriteFile(full, []byte(solutionPage), 0o644); err != nil {
		t.Fatal(err)
	}

	var steps any
	if err := json.Unmarshal([]byte(stepsJson), &steps); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePage(SaveRequest{Path: "content/solution.md", Set: map[string]any{"steps": steps}}); err != nil {
		t.Fatal(err)
	}

	saved, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	return string(saved)
}

func TestSavePageMergesNestedYaml(t *testing.T) {
	one := `{"title":"One","text":"Alpha","slides":[{"before":"a.jpg","after":"b.jpg"},{"before":"c.jpg","after":"d.jpg"}]}`
	two := `{"title":"Two","text":"Beta","slides":[]}`

	tests := map[string]struct {
		steps string
		want  string
	}{
		"changing one title": {
			steps: `[{"title":"Uno","text":"Alpha","slides":[{"before":"a.jpg","after":"b.jpg"},{"before":"c.jpg","after":"d.jpg"}]},` + two + `]`,
			want: `---
title: Solution
# the steps of the solution
steps:
  - title: Uno # first
    text: Alpha
    slides:
      - before: a.jpg
        after: b.jpg
      - before: c.jpg
        after: d.jpg
  # the second step
  - title: Two
    text: "Beta"
    slides: []
---
Body
`,
		},
		"adding an item": {
			steps: `[` + one + `,` + two + `,{"title":"Three","text":"Gamma","slides":[]}]`,
			want: `---
title: Solution
# the steps of the solution
steps:
  - title: One # first
    text: Alpha
    slides:
      - before: a.jpg
        after: b.jpg
      - before: c.jpg
        after: d.jpg
  # the second step
  - title: Two
    text: "Beta"
    slides: []
  - title: Three
    text: Gamma
    slides: []
---
Body
`,
		},
		"removing the last item": {
			steps: `[` + one + `]`,
			want: `---
title: Solution
# the steps of the solution
steps:
  - title: One # first
    text: Alpha
    slides:
      - before: a.jpg
        after: b.jpg
      - before: c.jpg
        after: d.jpg
---
Body
`,
		},
		"moving an item merges by position so comments stay put": {
			steps: `[` + two + `,` + one + `]`,
			want: `---
title: Solution
# the steps of the solution
steps:
  - title: Two # first
    text: Beta
    slides: []
  # the second step
  - title: One
    text: "Alpha"
    slides:
      - after: b.jpg
        before: a.jpg
      - after: d.jpg
        before: c.jpg
---
Body
`,
		},
		"emptying a nested list": {
			steps: `[{"title":"One","text":"Alpha","slides":[]},` + two + `]`,
			want: `---
title: Solution
# the steps of the solution
steps:
  - title: One # first
    text: Alpha
    slides: []
  # the second step
  - title: Two
    text: "Beta"
    slides: []
---
Body
`,
		},
		"filling an empty nested list": {
			steps: `[` + one + `,{"title":"Two","text":"Beta","slides":[{"before":"e.jpg","after":"f.jpg"}]}]`,
			want: `---
title: Solution
# the steps of the solution
steps:
  - title: One # first
    text: Alpha
    slides:
      - before: a.jpg
        after: b.jpg
      - before: c.jpg
        after: d.jpg
  # the second step
  - title: Two
    text: "Beta"
    slides:
      - after: f.jpg
        before: e.jpg
---
Body
`,
		},
		"adding and dropping keys in an item": {
			steps: `[{"title":"One","note":"hi","slides":[{"before":"a.jpg","after":"b.jpg"},{"before":"c.jpg","after":"d.jpg"}]},` + two + `]`,
			want: `---
title: Solution
# the steps of the solution
steps:
  - title: One # first
    slides:
      - before: a.jpg
        after: b.jpg
      - before: c.jpg
        after: d.jpg
    note: hi
  # the second step
  - title: Two
    text: "Beta"
    slides: []
---
Body
`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := saveSteps(t, test.steps); got != test.want {
				t.Errorf("want\n%s\ngot\n%s", test.want, got)
			}
		})
	}
}
