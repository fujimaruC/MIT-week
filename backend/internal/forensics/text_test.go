package forensics

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// strokes draws pseudo-lettering: a few dark bars in 14x22 cells, enough to
// look like text to TextLikeBlocks and to an ELA map.
func strokes(img *image.RGBA, x, y, cells, seed int, ink color.RGBA) image.Rectangle {
	for c := 0; c < cells; c++ {
		cx := x + c*16
		for k := 0; k < 3; k++ {
			v := (seed*31 + c*17 + k*7) % 5
			if k%2 == 0 {
				fillRect(img, image.Rect(cx+v*2, y, cx+v*2+3, y+22), ink) // vertical bar
			} else {
				fillRect(img, image.Rect(cx, y+v*4, cx+12, y+v*4+3), ink) // horizontal bar
			}
		}
	}
	return image.Rect(x, y, x+cells*16, y+22)
}

func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func recompress(t *testing.T, img image.Image, q int) *image.RGBA {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		t.Fatal(err)
	}
	out, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	rgba := image.NewRGBA(out.Bounds())
	for y := 0; y < out.Bounds().Dy(); y++ {
		for x := 0; x < out.Bounds().Dx(); x++ {
			rgba.Set(x, y, out.At(x, y))
		}
	}
	return rgba
}

func norm1000(r image.Rectangle, w, h int) [4]int {
	return [4]int{r.Min.Y * 1000 / h, r.Min.X * 1000 / w, r.Max.Y * 1000 / h, r.Max.X * 1000 / w}
}

// captionGraphic is a photo-like card with a white caption bar and three
// lines of lettering; it returns the image and the third line's box.
func captionGraphic() (*image.RGBA, image.Rectangle) {
	const w, h = 480, 360
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := uint8((x*7 + y*13 + (x*y)%23) % 40)
			img.SetRGBA(x, y, color.RGBA{70 + n, 110 + n/2, 90 + n, 255})
		}
	}
	fillRect(img, image.Rect(0, 200, w, h), color.RGBA{245, 245, 240, 255})
	ink := color.RGBA{20, 20, 20, 255}
	strokes(img, 24, 216, 22, 1, ink)
	strokes(img, 24, 256, 26, 2, ink)
	line := strokes(img, 24, 300, 14, 3, ink)
	return img, line
}

func TestTextEvidenceFindsARetypedLine(t *testing.T) {
	img, line := captionGraphic()
	// The original circulated: saved by the publisher, recompressed by a
	// messenger.
	legit := recompress(t, recompress(t, img, 80), 75)

	// The hoax: the third line covered and retyped, saved by an editor.
	hoax := image.NewRGBA(legit.Bounds())
	copy(hoax.Pix, legit.Pix)
	fillRect(hoax, line.Inset(-3), color.RGBA{245, 245, 240, 255})
	retyped := strokes(hoax, 24, 300, 14, 9, color.RGBA{20, 20, 20, 255})
	hoax = recompress(t, hoax, 95)

	w, h := legit.Bounds().Dx(), legit.Bounds().Dy()
	got := TextEvidence(hoax, [][4]int{norm1000(retyped, w, h)})[0]
	base := TextEvidence(legit, [][4]int{norm1000(line, w, h)})[0]
	if got < 70 {
		t.Errorf("retyped line evidence = %d, want >= 70", got)
	}
	if base < 0 || base >= 50 {
		t.Errorf("original line evidence = %d, want 0..49", base)
	}
}

func TestTextEvidenceCannotAlwaysMeasure(t *testing.T) {
	img, line := captionGraphic()
	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	// A box with no lettering in it.
	empty := image.Rect(300, 100, 460, 140)
	// A box on an image with no other text to compare with.
	plain := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(plain, plain.Bounds(), color.RGBA{200, 200, 200, 255})
	only := strokes(plain, 24, 300, 14, 3, color.RGBA{20, 20, 20, 255})

	got := TextEvidence(img, [][4]int{norm1000(empty, w, h), {0, 0, 0, 0}})
	if got[0] != -1 || got[1] != -1 {
		t.Errorf("unmeasurable boxes = %v, want -1", got)
	}
	if got := TextEvidence(plain, [][4]int{norm1000(only, w, h)})[0]; got != -1 {
		t.Errorf("no reference text: evidence = %d, want -1", got)
	}
	tiny := image.NewRGBA(image.Rect(0, 0, 40, 40))
	if got := TextEvidence(tiny, [][4]int{{0, 0, 1000, 1000}})[0]; got != -1 {
		t.Errorf("tiny image: evidence = %d, want -1", got)
	}
	_ = line
}

func TestTextLikeBlocksIgnorePhotoTexture(t *testing.T) {
	img, _ := captionGraphic()
	lum, grad := luminance(img)
	w, h := img.Rect.Dx(), img.Rect.Dy()
	blocks := TextLikeBlocks(lum, grad, w, h, image.Rectangle{})
	if len(blocks) < 8 {
		t.Fatalf("found %d text-like blocks in the caption, want several", len(blocks))
	}
	for _, b := range blocks {
		if b.Max.Y <= 200 {
			t.Fatalf("photo texture %v taken for lettering", b)
		}
	}
}
