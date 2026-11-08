package hugo

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// A macOS .pkg is a xar archive whose "Payload" is a gzip-compressed cpio
// archive. extractPkg reads the hugo program straight out of it. No installer
// or script runs, and nothing is written outside Pageroom's cache.

type xarFile struct {
	Name string `xml:"name"`
	Type string `xml:"type"`
	Data struct {
		Offset   int64 `xml:"offset"`
		Length   int64 `xml:"length"`
		Size     int64 `xml:"size"`
		Encoding struct {
			Style string `xml:"style,attr"`
		} `xml:"encoding"`
	} `xml:"data"`
	Files []xarFile `xml:"file"`
}

const maxPayload = 600 << 20

func extractPkg(archive []byte) ([]byte, error) {
	if len(archive) < 28 || string(archive[:4]) != "xar!" {
		return nil, errors.New("not a macOS installer package")
	}
	headerSize := int64(binary.BigEndian.Uint16(archive[4:6]))
	tocLen := int64(binary.BigEndian.Uint64(archive[8:16]))
	if headerSize < 28 || headerSize+tocLen > int64(len(archive)) {
		return nil, errors.New("damaged installer package")
	}
	zr, err := zlib.NewReader(bytes.NewReader(archive[headerSize : headerSize+tocLen]))
	if err != nil {
		return nil, err
	}
	tocXML, err := io.ReadAll(io.LimitReader(zr, 16<<20))
	if err != nil {
		return nil, err
	}
	var toc struct {
		Files []xarFile `xml:"toc>file"`
	}
	if err := xml.Unmarshal(tocXML, &toc); err != nil {
		return nil, fmt.Errorf("damaged installer package: %w", err)
	}
	payload := findXar(toc.Files, "Payload")
	if payload == nil {
		return nil, errors.New("installer package has no payload")
	}
	heap := headerSize + tocLen
	start, end := heap+payload.Data.Offset, heap+payload.Data.Offset+payload.Data.Length
	if payload.Data.Offset < 0 || payload.Data.Length < 0 || end > int64(len(archive)) || start > end {
		return nil, errors.New("damaged installer package")
	}
	data := archive[start:end]
	// xar may compress the entry itself (zlib). The payload inside is gzip.
	if strings.Contains(payload.Data.Encoding.Style, "gzip") {
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if data, err = io.ReadAll(io.LimitReader(r, maxPayload)); err != nil {
			return nil, err
		}
	}
	if bytes.HasPrefix(data, []byte("pbzx")) {
		return nil, errors.New("this Hugo package uses a compression Pageroom can't read yet; please install Hugo yourself")
	}
	if bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if data, err = io.ReadAll(io.LimitReader(gz, maxPayload)); err != nil {
			return nil, err
		}
	}
	return cpioFile(data, "hugo")
}

func findXar(files []xarFile, name string) *xarFile {
	for i := range files {
		if files[i].Name == name && files[i].Type == "file" {
			return &files[i]
		}
		if f := findXar(files[i].Files, name); f != nil {
			return f
		}
	}
	return nil
}

// cpioFile returns the regular file called want (at any depth) from an
// "odc" (070707) or "newc" (070701) cpio archive.
func cpioFile(b []byte, want string) ([]byte, error) {
	for i := 0; i < len(b); {
		var mode, nameSize, fileSize int64
		var hdr int
		var err error
		switch {
		case bytes.HasPrefix(b[i:], []byte("070707")):
			if i+76 > len(b) {
				return nil, errors.New("damaged package payload")
			}
			h := b[i : i+76]
			mode, err = parseNum(h[18:24], 8)
			if err == nil {
				nameSize, err = parseNum(h[59:65], 8)
			}
			if err == nil {
				fileSize, err = parseNum(h[65:76], 8)
			}
			hdr = 76
		case bytes.HasPrefix(b[i:], []byte("070701")):
			if i+110 > len(b) {
				return nil, errors.New("damaged package payload")
			}
			h := b[i : i+110]
			mode, err = parseNum(h[14:22], 16)
			if err == nil {
				fileSize, err = parseNum(h[54:62], 16)
			}
			if err == nil {
				nameSize, err = parseNum(h[94:102], 16)
			}
			hdr = 110
		default:
			return nil, errors.New("unknown package payload format")
		}
		if err != nil || nameSize <= 0 || fileSize < 0 {
			return nil, errors.New("damaged package payload")
		}
		nameStart := i + hdr
		nameEnd := nameStart + int(nameSize)
		dataStart := nameEnd
		if hdr == 110 { // newc pads header+name and data to 4 bytes
			dataStart = (nameEnd + 3) &^ 3
		}
		dataEnd := dataStart + int(fileSize)
		if nameEnd > len(b) || dataEnd > len(b) {
			return nil, errors.New("damaged package payload")
		}
		name := strings.TrimRight(string(b[nameStart:nameEnd]), "\x00")
		if name == "TRAILER!!!" {
			break
		}
		if path.Base(name) == want && mode&0o170000 == 0o100000 {
			return b[dataStart:dataEnd], nil
		}
		i = dataEnd
		if hdr == 110 {
			i = (i + 3) &^ 3
		}
	}
	return nil, errors.New("hugo program not found in the package")
}

func parseNum(b []byte, base int) (int64, error) {
	return strconv.ParseInt(string(b), base, 64)
}
