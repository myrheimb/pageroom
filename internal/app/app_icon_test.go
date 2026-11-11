package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrheimb/pageroom/internal/favicon"
)

func TestUpdateLetterIcon(t *testing.T) {
	letter := func(title string) []byte {
		icons, _ := favicon.Letter(title, favicon.DefaultBackground)
		return icons.Favicon
	}
	teammates := append(letter("Bakery"), 0)
	chosen := []byte("chosen icon")
	tests := []struct {
		name  string
		site  []byte // static/favicon.ico before publishing, nil for none
		theme []byte
		want  []byte // nil for no file
	}{
		{"no icon gets the letter", nil, nil, letter("Bakery")},
		{"a renamed site gets its new letter", letter("Cakes"), nil, letter("Bakery")},
		{"a teammate's drawing of the same letter stays", teammates, nil, teammates},
		{"a chosen icon stays", chosen, nil, chosen},
		{"the theme's icon stays", nil, chosen, nil},
		{"a letter replaces the theme's icon", letter("Cakes"), chosen, letter("Bakery")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, _ := newLocalSite(t, nil)
			root := a.site.Root
			write := func(path string, content []byte) {
				if content == nil {
					return
				}
				_ = os.MkdirAll(filepath.Dir(path), 0o755)
				if err := os.WriteFile(path, content, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(root, favicon.File), test.site)
			write(filepath.Join(root, "themes", a.site.Config.Theme(), favicon.File), test.theme)

			if err := a.updateLetterIcon(a.site); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(root, favicon.File))
			if test.want == nil && !os.IsNotExist(err) {
				t.Fatalf("wrote an icon over the theme's: %v", err)
			}
			if test.want != nil && !bytes.Equal(got, test.want) {
				t.Fatalf("got %d bytes, want %d", len(got), len(test.want))
			}
			_, err = os.Stat(filepath.Join(root, favicon.TouchFile))
			if wrote := !bytes.Equal(test.site, test.want); wrote == os.IsNotExist(err) {
				t.Fatalf("wrote the favicon: %v, touch icon: %v", wrote, err)
			}
		})
	}
}
