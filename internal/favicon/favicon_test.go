package favicon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"
)

// decode returns the favicon's images, then the touch icon.
func decode(t *testing.T, icons *Icons) []image.Image {
	t.Helper()
	icon := icons.Favicon
	var header [3]uint16
	if err := binary.Read(bytes.NewReader(icon), binary.LittleEndian, &header); err != nil || header[1] != 1 {
		t.Fatalf("not an ICO file: %v %v", header, err)
	}

	var images []image.Image
	for index := range int(header[2]) {
		entry := icon[6+16*index:]
		size := binary.LittleEndian.Uint32(entry[8:])
		offset := binary.LittleEndian.Uint32(entry[12:])
		picture, err := png.Decode(bytes.NewReader(icon[offset : offset+size]))
		if err != nil {
			t.Fatalf("image %d: %v", index, err)
		}
		if side := int(entry[0]); picture.Bounds().Dx() != side || picture.Bounds().Dy() != side {
			t.Fatalf("image %d is %v, directory says %d", index, picture.Bounds(), side)
		}
		images = append(images, picture)
	}
	if len(images) != len(sizes) || images[0].Bounds().Dx() != 32 {
		t.Fatalf("got %d images, smallest %v", len(images), images[0].Bounds())
	}

	touch, err := png.Decode(bytes.NewReader(icons.Touch))
	if err != nil {
		t.Fatalf("touch icon: %v", err)
	}
	if touch.Bounds().Dx() != touchSize || touch.Bounds().Dy() != touchSize {
		t.Fatalf("touch icon is %v", touch.Bounds())
	}
	return append(images, touch)
}

func inked(picture image.Image, background color.Color) bool {
	want := color.RGBAModel.Convert(background)
	bounds := picture.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if color.RGBAModel.Convert(picture.At(x, y)) != want {
				return true
			}
		}
	}
	return false
}

func TestLetter(t *testing.T) {
	slate := color.RGBA{0x33, 0x41, 0x55, 0xff}
	tests := []struct {
		title      string
		background string
		fill       color.Color
		inked      bool
	}{
		{"catcakes", DefaultBackground, slate, true},
		{"  — 2026 bakes", "#fff", color.White, true},
		{"", "#ff000080", color.RGBA{0xff, 0, 0, 0xff}, false},
		{"…", DefaultBackground, slate, false},
		{"ねこカフェ", DefaultBackground, slate, runtime.GOOS == "darwin"},
		{"猫", DefaultBackground, slate, runtime.GOOS == "darwin"},
	}
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			icons, err := Letter(test.title, test.background)
			if err != nil {
				t.Fatal(err)
			}
			if !IsLetter(icons.Favicon) || !SameLetter(icons.Favicon, test.title, test.background) {
				t.Fatal("not marked as this letter icon")
			}
			for _, picture := range decode(t, icons) {
				if corner := color.RGBAModel.Convert(picture.At(0, 0)); corner != color.RGBAModel.Convert(test.fill) {
					t.Fatalf("background is %v", corner)
				}
				if inked(picture, test.fill) != test.inked {
					t.Fatalf("drawn: %v, want %v", !test.inked, test.inked)
				}
			}
		})
	}
}

func TestSameLetter(t *testing.T) {
	icons, _ := Letter("catcakes", "#334155")
	tests := []struct {
		title      string
		background string
		same       bool
	}{
		{"Cat café", "#334155", true},
		{"catcakes", "#334156", false},
		{"dogcakes", "#334155", false},
		{"", "#334155", false},
	}
	for _, test := range tests {
		if SameLetter(icons.Favicon, test.title, test.background) != test.same {
			t.Errorf("%q on %s: want same %v", test.title, test.background, test.same)
		}
	}
}

func TestLetterRejectsBadColor(t *testing.T) {
	for _, background := range []string{"", "red", "#12345", "#gggggg"} {
		if _, err := Letter("a", background); err == nil {
			t.Errorf("accepted %q", background)
		}
	}
}

func TestFromImage(t *testing.T) {
	wide := image.NewRGBA(image.Rect(0, 0, 300, 100))
	for x := range 300 {
		for y := range 100 {
			wide.Set(x, y, color.RGBA{uint8(x * 255 / 299), 0, 0, 0xff})
		}
	}
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, wide)

	icons, err := FromImage(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if IsLetter(icons.Favicon) {
		t.Fatal("a chosen image is marked as a letter icon")
	}
	for _, picture := range decode(t, icons) {
		left, _, _, _ := picture.At(0, 0).RGBA()
		right, _, _, _ := picture.At(picture.Bounds().Dx()-1, 0).RGBA()
		if left>>8 < 70 || right>>8 > 185 {
			t.Fatalf("expected the middle third, got red %d to %d", left>>8, right>>8)
		}
	}

	encoded.Reset()
	_ = png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 64, 64)))
	icons, err = FromImage(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	pictures := decode(t, icons)
	if _, _, _, alpha := pictures[0].At(0, 0).RGBA(); alpha != 0 {
		t.Fatal("the favicon lost its transparency")
	}
	if corner := color.RGBAModel.Convert(pictures[len(pictures)-1].At(0, 0)); corner != color.RGBAModel.Convert(color.White) {
		t.Fatalf("the touch icon's transparent parts are %v, want white", corner)
	}

	if _, err := FromImage([]byte("<svg></svg>")); err == nil {
		t.Fatal("accepted an SVG")
	}
}
