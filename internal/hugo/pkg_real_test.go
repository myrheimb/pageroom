package hugo

import (
	"bytes"
	"os"
	"testing"
)

// Set PAGEROOM_TEST_PKG to a real Hugo macOS .pkg to check extraction
// against Hugo's actual release format.
func TestExtractRealPkg(t *testing.T) {
	p := os.Getenv("PAGEROOM_TEST_PKG")
	if p == "" {
		t.Skip("PAGEROOM_TEST_PKG not set")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	bin, err := extractPkg(b)
	if err != nil {
		t.Fatal(err)
	}
	// Mach-O universal binary magic
	if !bytes.HasPrefix(bin, []byte{0xca, 0xfe, 0xba, 0xbe}) || len(bin) < 50<<20 {
		t.Fatalf("not a macOS universal binary: % x, %d bytes", bin[:4], len(bin))
	}
}
