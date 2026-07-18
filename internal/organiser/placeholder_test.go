package organiser

import (
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePlaceholderPoster(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "poster.jpg")
	if err := writePlaceholderPoster(dest, "SSIS-001"); err != nil {
		t.Fatalf("writePlaceholderPoster: %v", err)
	}

	f, err := os.Open(dest)
	if err != nil {
		t.Fatalf("opening generated poster: %v", err)
	}
	defer f.Close()

	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("decoding generated poster as jpeg: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 600 {
		t.Fatalf("unexpected dimensions %dx%d", b.Dx(), b.Dy())
	}
}
