package organiser

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"
)

func encodeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding test jpeg: %v", err)
	}
	return buf.Bytes()
}

func TestCropFrontCover(t *testing.T) {
	cases := []struct {
		name          string
		w, h          int
		wantCropW     int
		wantUncropped bool
	}{
		// Matches the two real samples verified against a known-good
		// reference: 800x538 (S1/JavBus) and 800x438 (JavDB), same title.
		{name: "800x538", w: 800, h: 538, wantCropW: 378},
		{name: "800x438", w: 800, h: 438, wantCropW: 307},
		{name: "already narrow", w: 300, h: 538, wantUncropped: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := encodeTestJPEG(t, c.w, c.h)
			cropped, err := cropFrontCover(raw)
			if c.wantUncropped {
				if err == nil {
					t.Fatalf("expected error for a %dx%d image (narrower than the crop), got none", c.w, c.h)
				}
				return
			}
			if err != nil {
				t.Fatalf("cropFrontCover: %v", err)
			}
			img, err := jpeg.Decode(bytes.NewReader(cropped))
			if err != nil {
				t.Fatalf("decoding cropped output: %v", err)
			}
			b := img.Bounds()
			if b.Dx() != c.wantCropW || b.Dy() != c.h {
				t.Fatalf("got %dx%d, want %dx%d", b.Dx(), b.Dy(), c.wantCropW, c.h)
			}
		})
	}
}

func TestCropFrontCoverInvalidImage(t *testing.T) {
	if _, err := cropFrontCover([]byte("not an image")); err == nil {
		t.Fatal("expected an error for undecodable input, got none")
	}
}
