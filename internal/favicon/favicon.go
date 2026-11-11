// Package favicon makes the icons for browser tabs, bookmarks and the iPhone
// home screen, from an image the owner picks or the first letter of the
// title.
package favicon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"
)

// File and TouchFile are where a Hugo site keeps its icons. Browsers ask for
// /favicon.ico and iOS for /apple-touch-icon.png when a page names no icon.
const (
	File      = "static/favicon.ico"
	TouchFile = "static/apple-touch-icon.png"
)

var sizes = []int{32, 48}

const touchSize = 180

type Icons struct {
	Favicon, Touch []byte
}

// letterMarker tells a letter icon from a chosen one, so only letter icons
// follow the title. Each icon also records what it was drawn from, because
// computers draw letters with different fonts and comparing the drawings
// would change the icon on every publish.
const letterMarker = "Pageroom letter icon"

// DefaultBackground is used when the site has no primary color.
const DefaultBackground = "#334155"

// FromImage makes an icon from the middle square of a JPEG, PNG, GIF or
// WebP image.
func FromImage(data []byte) (*Icons, error) {
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("this file isn't a JPEG, PNG, GIF or WebP image")
	}

	bounds := source.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	corner := bounds.Min.Add(image.Pt((bounds.Dx()-side)/2, (bounds.Dy()-side)/2))
	square := image.Rectangle{Min: corner, Max: corner.Add(image.Pt(side, side))}
	return render(func(size int) (image.Image, error) {
		scaled := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), source, square, draw.Src, nil)
		return scaled, nil
	}, "")
}

// Letter draws the first letter or digit of title on background, a color
// like #1a2b3c. Letters without a font on this computer get a plain square.
func Letter(title, background string) (*Icons, error) {
	fill, err := parseColor(background)
	if err != nil {
		return nil, err
	}

	ink := color.Color(color.White)
	red, green, blue, _ := fill.RGBA()
	if 299*red+587*green+114*blue > 1000*0xffff*6/10 {
		ink = color.Black
	}
	character := firstCharacter(title)
	typeface := fontFor(character)
	return render(func(size int) (image.Image, error) {
		canvas := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.Draw(canvas, canvas.Bounds(), image.NewUniform(fill), image.Point{}, draw.Src)
		if typeface != nil {
			if err := drawCentered(canvas, typeface, character, ink); err != nil {
				return nil, err
			}
		}
		return canvas, nil
	}, letterNote(title, background))
}

func IsLetter(icon []byte) bool {
	return bytes.Contains(icon, []byte(letterMarker))
}

// SameLetter reports whether Letter would draw icon again from title and
// background.
func SameLetter(icon []byte, title, background string) bool {
	return bytes.Contains(icon, []byte(letterNote(title, background)))
}

func letterNote(title, background string) string {
	return fmt.Sprintf("%s U+%04X %s", letterMarker, firstCharacter(title), strings.ToLower(background))
}

func firstCharacter(title string) rune {
	for _, character := range title {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToUpper(character)
		}
	}
	return 0
}

// systemFonts cover Japanese, which the Go fonts don't. Pageroom runs on
// the owner's computer, so it can borrow the fonts already there.
var systemFonts = map[string][]string{
	"darwin":  {"/System/Library/Fonts/ヒラギノ角ゴシック W6.ttc"},
	"linux":   {"/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc", "/usr/share/fonts/noto-cjk/NotoSansCJK-Bold.ttc", "/usr/share/fonts/google-noto-cjk/NotoSansCJK-Bold.ttc"},
	"windows": {`C:\Windows\Fonts\YuGothB.ttc`, `C:\Windows\Fonts\meiryob.ttc`},
}

func fontFor(character rune) *sfnt.Font {
	if character == 0 {
		return nil
	}

	if typeface := withGlyph(gobold.TTF, character); typeface != nil {
		return typeface
	}
	for _, path := range systemFonts[runtime.GOOS] {
		if data, err := os.ReadFile(path); err == nil {
			if typeface := withGlyph(data, character); typeface != nil {
				return typeface
			}
		}
	}
	return nil
}

func withGlyph(data []byte, character rune) *sfnt.Font {
	collection, err := sfnt.ParseCollection(data)
	if err != nil {
		return nil
	}
	typeface, err := collection.Font(0)
	if err != nil {
		return nil
	}
	if glyph, err := typeface.GlyphIndex(&sfnt.Buffer{}, character); err != nil || glyph == 0 {
		return nil
	}
	return typeface
}

// drawCentered sizes the character so its widest side fills 60% of the
// canvas, whatever the script.
func drawCentered(canvas *image.RGBA, typeface *sfnt.Font, character rune, ink color.Color) error {
	side := float64(canvas.Bounds().Dx())
	face, err := opentype.NewFace(typeface, &opentype.FaceOptions{Size: side, DPI: 72})
	if err != nil {
		return err
	}

	bounds, _, _ := face.GlyphBounds(character)
	extent := max(bounds.Max.X-bounds.Min.X, bounds.Max.Y-bounds.Min.Y)
	if extent <= 0 {
		return nil
	}
	face, err = opentype.NewFace(typeface, &opentype.FaceOptions{Size: side * side * 0.6 / (float64(extent) / 64), DPI: 72})
	if err != nil {
		return err
	}

	bounds, _, _ = face.GlyphBounds(character)
	middle := fixed.I(canvas.Bounds().Dx()) / 2
	drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(ink), Face: face, Dot: fixed.Point26_6{
		X: middle - (bounds.Min.X+bounds.Max.X)/2,
		Y: middle - (bounds.Min.Y+bounds.Max.Y)/2,
	}}
	drawer.DrawString(string(character))
	return nil
}

func parseColor(hex string) (color.RGBA, error) {
	digits := strings.TrimPrefix(hex, "#")
	if len(digits) == 8 {
		digits = digits[:6]
	}
	if len(digits) == 3 {
		digits = string([]byte{digits[0], digits[0], digits[1], digits[1], digits[2], digits[2]})
	}
	value, err := strconv.ParseUint(digits, 16, 32)
	if err != nil || len(digits) != 6 {
		return color.RGBA{}, errors.New("color must look like #1a2b3c")
	}
	return color.RGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: 0xff}, nil
}

// render draws both icons. The touch icon goes on white, because iOS fills
// transparent parts with black.
func render(paint func(size int) (image.Image, error), note string) (*Icons, error) {
	images := make([]image.Image, len(sizes))
	for index, size := range sizes {
		picture, err := paint(size)
		if err != nil {
			return nil, err
		}
		images[index] = picture
	}
	favicon, err := encode(images, note)
	if err != nil {
		return nil, err
	}

	picture, err := paint(touchSize)
	if err != nil {
		return nil, err
	}
	opaque := image.NewRGBA(picture.Bounds())
	draw.Draw(opaque, opaque.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(opaque, opaque.Bounds(), picture, image.Point{}, draw.Over)
	var touch bytes.Buffer
	if err := png.Encode(&touch, opaque); err != nil {
		return nil, err
	}
	return &Icons{Favicon: favicon, Touch: touch.Bytes()}, nil
}

// encode writes an ICO file holding PNG images, which every current
// browser reads. A note is stored as a PNG text chunk.
func encode(images []image.Image, note string) ([]byte, error) {
	entries := make([][]byte, len(images))
	for index, picture := range images {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, picture); err != nil {
			return nil, err
		}
		entries[index] = withText(buffer.Bytes(), note)
	}

	var icon bytes.Buffer
	_ = binary.Write(&icon, binary.LittleEndian, [3]uint16{0, 1, uint16(len(entries))})
	offset := 6 + 16*len(entries)
	for index, entry := range entries {
		side := uint8(images[index].Bounds().Dx())
		_ = binary.Write(&icon, binary.LittleEndian, struct {
			Width, Height, Colors, Reserved uint8
			Planes, BitsPerPixel            uint16
			Size, Offset                    uint32
		}{side, side, 0, 0, 1, 32, uint32(len(entry)), uint32(offset)})
		offset += len(entry)
	}
	for _, entry := range entries {
		icon.Write(entry)
	}
	return icon.Bytes(), nil
}

func withText(encoded []byte, note string) []byte {
	if note == "" {
		return encoded
	}

	const headerEnd = 8 + 4 + 4 + 13 + 4 // signature, then the IHDR chunk
	body := []byte("tEXtSoftware\x00" + note)
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(body)-4))
	chunk = append(chunk, body...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(body))
	return slices.Concat(encoded[:headerEnd], chunk, encoded[headerEnd:])
}
