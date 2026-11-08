package frontmatter

import (
	"strings"
	"testing"
	"time"
)

var samples = map[string]string{
	"yaml": "---\ntitle: Hello   # greeting\ntags: [a, b]\ndraft: true\n---\n\nBody *text*\n",
	"toml": "+++\n# site post\ntitle = \"Hello\"\ntags = [\n  \"a\",\n  \"b\",\n]\n\n[params]\n  hero = \"x.jpg\"\n+++\nBody\n",
	"json": "{\n    \"title\": \"Hello\",\n    \"zeta\": 1,\n    \"alpha\": 2\n}\n\nBody\n",
	"none": "Just a body\n",
	"crlf": "---\r\ntitle: Windows\r\n---\r\nLine one\r\nLine two\r\n",
	"bom":  "\xEF\xBB\xBF+++\ntitle = 'x'\n+++\nhi",
}

func TestRoundTripIsByteIdentical(t *testing.T) {
	for name, src := range samples {
		d, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out, err := d.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != src {
			t.Errorf("%s: round trip changed file\nwant %q\ngot  %q", name, src, out)
		}
	}
}

func TestSettingSameValueKeepsBytes(t *testing.T) {
	d, _ := Parse([]byte(samples["yaml"]))
	d.Set("title", "Hello", YAML)
	out, _ := d.Bytes()
	if string(out) != samples["yaml"] {
		t.Fatalf("no-op set rewrote file: %q", out)
	}
}

func TestYAMLEditKeepsCommentsAndOrder(t *testing.T) {
	d, _ := Parse([]byte(samples["yaml"]))
	if err := d.Set("title", "Goodbye", YAML); err != nil {
		t.Fatal(err)
	}
	d.Set("weight", 3, YAML)
	out, _ := d.Bytes()
	s := string(out)
	if !strings.Contains(s, "title: Goodbye # greeting") {
		t.Errorf("comment lost: %s", s)
	}
	if strings.Index(s, "title") > strings.Index(s, "draft") || !strings.Contains(s, "weight: 3") {
		t.Errorf("order wrong: %s", s)
	}
	if !strings.HasSuffix(s, "---\n\nBody *text*\n") {
		t.Errorf("body changed: %q", s)
	}
}

func TestTOMLEdits(t *testing.T) {
	d, err := Parse([]byte(samples["toml"]))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Keys(); strings.Join(got, ",") != "title,tags,params" {
		t.Fatalf("keys = %v", got)
	}
	d.Set("tags", []string{"x"}, TOML)
	d.Set("draft", true, TOML)
	d.Set("params", map[string]any{"hero": "y.jpg"}, TOML)
	out, _ := d.Bytes()
	s := string(out)
	for _, want := range []string{"# site post", "title = \"Hello\"", "draft = true", "hero = 'y.jpg'"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\"a\"") {
		t.Errorf("old multi-line array not replaced:\n%s", s)
	}
	if strings.Index(s, "draft") > strings.Index(s, "[params]") {
		t.Errorf("new key must go before tables:\n%s", s)
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("result does not reparse: %v", err)
	}
	if v, _ := d2.Get("params"); v.(map[string]any)["hero"] != "y.jpg" {
		t.Errorf("params = %v", v)
	}
}

func TestTOMLDeleteTable(t *testing.T) {
	d, _ := Parse([]byte(samples["toml"]))
	d.Delete("params")
	out, _ := d.Bytes()
	if strings.Contains(string(out), "params") || strings.Contains(string(out), "hero") {
		t.Fatalf("table not removed:\n%s", out)
	}
}

func TestJSONKeepsOrder(t *testing.T) {
	d, _ := Parse([]byte(samples["json"]))
	d.Set("beta", "b", JSON)
	out, _ := d.Bytes()
	s := string(out)
	if !(strings.Index(s, "zeta") < strings.Index(s, "alpha") && strings.Index(s, "alpha") < strings.Index(s, "beta")) {
		t.Fatalf("order lost:\n%s", s)
	}
}

func TestAddFrontMatterToPlainFile(t *testing.T) {
	d, _ := Parse([]byte(samples["none"]))
	d.Set("title", "New", YAML)
	out, _ := d.Bytes()
	if string(out) != "---\ntitle: New\n---\nJust a body\n" {
		t.Fatalf("got %q", out)
	}
}

func TestCRLFPreserved(t *testing.T) {
	d, _ := Parse([]byte(samples["crlf"]))
	d.SetBody([]byte("Changed\n"))
	out, _ := d.Bytes()
	if string(out) != "---\r\ntitle: Windows\r\n---\r\nChanged\r\n" {
		t.Fatalf("got %q", out)
	}
}

func TestDates(t *testing.T) {
	ts := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	for _, f := range []Format{YAML, TOML} {
		d, _ := Parse(nil)
		d.Set("date", ts, f)
		out, _ := d.Bytes()
		if !strings.Contains(string(out), "2026-03-01T09:30:00Z") {
			t.Errorf("%s date encoding: %s", f, out)
		}
	}
}

func TestBrokenFrontMatterIsReported(t *testing.T) {
	for _, src := range []string{"---\ntitle: [unclosed\n---\n", "+++\ntitle = \n+++\n", "---\nno close"} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}

func TestParseDataConfig(t *testing.T) {
	src := "# My site\nbaseURL = 'https://example.org/'\ntitle = 'Old'\n\n[params]\n  description = 'x'\n"
	d, err := ParseData([]byte(src), TOML)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := d.Bytes()
	if string(out) != src {
		t.Fatalf("round trip: %q", out)
	}
	d.Set("title", "New", TOML)
	out, _ = d.Bytes()
	if !strings.Contains(string(out), "# My site") || !strings.Contains(string(out), "title = 'New'") || strings.HasPrefix(string(out), "+++") {
		t.Fatalf("got %s", out)
	}
}
