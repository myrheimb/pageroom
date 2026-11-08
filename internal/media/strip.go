package media

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// stripJPEG removes metadata segments (EXIF with GPS, XMP, IPTC, comments)
// without re-encoding. It keeps JFIF, the ICC profile and Adobe markers so
// colors stay the same, and returns the EXIF orientation.
func stripJPEG(b []byte) (out []byte, orientation int, err error) {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return nil, 0, errors.New("not a JPEG")
	}
	orientation = 1
	var buf bytes.Buffer
	buf.Write(b[:2])
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return nil, 0, errors.New("corrupt JPEG")
		}
		marker := b[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		if marker == 0xDA { // start of scan: rest is image data
			buf.Write(b[i:])
			return buf.Bytes(), orientation, nil
		}
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			buf.Write(b[i : i+2])
			i += 2
			continue
		}
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		end := i + 2 + size
		if size < 2 || end > len(b) {
			return nil, 0, errors.New("corrupt JPEG")
		}
		seg := b[i+4 : end]
		keep := true
		switch {
		case marker == 0xE1: // APP1: EXIF or XMP
			if bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
				orientation = exifOrientation(seg[6:])
			}
			keep = false
		case marker == 0xE0: // APP0 JFIF: keep
		case marker == 0xE2: // APP2: keep ICC profile only
			keep = bytes.HasPrefix(seg, []byte("ICC_PROFILE\x00"))
		case marker == 0xEE: // APP14 Adobe: affects color decoding, keep
		case marker >= 0xE3 && marker <= 0xEF: // other APPn (IPTC, maker notes...)
			keep = false
		case marker == 0xFE: // comment
			keep = false
		}
		if keep {
			buf.Write(b[i:end])
		}
		i = end
	}
	return nil, 0, errors.New("corrupt JPEG")
}

// exifOrientation reads tag 0x0112 from a TIFF-structured EXIF block.
func exifOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:8]))
	if off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off : off+2]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:e+2]) == 0x0112 {
			v := int(bo.Uint16(t[e+8 : e+10]))
			if v >= 1 && v <= 8 {
				return v
			}
		}
	}
	return 1
}

var pngSig = []byte("\x89PNG\r\n\x1a\n")

// stripPNG drops text, EXIF and timestamp chunks, keeping color information.
func stripPNG(b []byte) ([]byte, error) {
	if !bytes.HasPrefix(b, pngSig) {
		return nil, errors.New("not a PNG")
	}
	drop := map[string]bool{"tEXt": true, "zTXt": true, "iTXt": true, "eXIf": true, "tIME": true}
	var buf bytes.Buffer
	buf.Write(pngSig)
	i := len(pngSig)
	for i+12 <= len(b) {
		n := int(binary.BigEndian.Uint32(b[i : i+4]))
		end := i + 12 + n
		if n < 0 || end > len(b) {
			return nil, errors.New("corrupt PNG")
		}
		typ := string(b[i+4 : i+8])
		if !drop[typ] {
			buf.Write(b[i:end])
		}
		i = end
		if typ == "IEND" {
			break
		}
	}
	return buf.Bytes(), nil
}

// stripWebP removes EXIF and XMP chunks from a RIFF WebP container and
// clears the matching flags in the VP8X header.
func stripWebP(b []byte) ([]byte, error) {
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return nil, errors.New("not a WebP")
	}
	var body bytes.Buffer
	i := 12
	for i+8 <= len(b) {
		fourcc := string(b[i : i+4])
		n := int(binary.LittleEndian.Uint32(b[i+4 : i+8]))
		end := i + 8 + n + n%2
		if end > len(b) {
			end = len(b)
		}
		switch fourcc {
		case "EXIF", "XMP ":
		case "VP8X":
			chunk := append([]byte(nil), b[i:end]...)
			if len(chunk) > 8 {
				chunk[8] &^= 0x08 | 0x04 // EXIF and XMP flags
			}
			body.Write(chunk)
		default:
			body.Write(b[i:end])
		}
		i = end
	}
	out := make([]byte, 12, 12+body.Len())
	copy(out, b[:12])
	binary.LittleEndian.PutUint32(out[4:8], uint32(4+body.Len()))
	return append(out, body.Bytes()...), nil
}
