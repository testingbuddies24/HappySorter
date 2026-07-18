package organiser

import (
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// writePlaceholderPoster generates a plain poster.jpg with code rendered on
// a solid background, for use when a source returns no cover image (or the
// download fails) — SPEC.md F5.
func writePlaceholderPoster(dest, code string) error {
	const w, h = 400, 600
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{R: 0x2b, G: 0x2b, B: 0x36, A: 0xff}
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	face := basicfont.Face7x13
	textW := font.MeasureString(face, code).Round()
	x := (w - textW) / 2
	y := h / 2

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.White),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(code)

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	return jpeg.Encode(f, img, &jpeg.Options{Quality: 85})
}
