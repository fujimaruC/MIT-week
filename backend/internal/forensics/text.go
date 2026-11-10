package forensics

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	"slices"
)

// Timpa Teks: was this text pasted over the image after it was made?
//
// A news graphic's text is all drawn at the same time, so every line has the
// same compression history. Text typed over a circulated image (a new price,
// a new date) was compressed fewer times, so it reacts more strongly to one
// more JPEG compression than the text around it. The global ELA mean cannot
// see this (it is dominated by the photo), so TextEvidence compares the
// suspicious text with the other text of the same image, normalised by edge
// strength so that bold or busy lettering is not mistaken for an edit.
//
// Measured in November 2026 on graphics built from 34 real camera photos: a
// caption bar saved by a publisher and recompressed by a messenger, and the
// same graphic with its price line covered and retyped in an editor, then
// forwarded once more for about half of them. The thresholds were set on one
// set of 34 pairs and checked on a second, generated independently. Original
// lines: none reached evidence 50 in either set. Retyped lines: 65 % and 41 %
// reached 50 (62 % and 32 % reached 70); 71 % and 50 % when the edit was not
// forwarded again, 54 % and 31 % when it was. Boxes 30 % too large or shifted
// by a tenth gave the same picture. So high evidence is a strong sign, and
// low evidence says little: a forward or a resize can erase the difference.
const (
	textBlock = 16 // MCU size with 4:2:0 chroma: crops stay grid-aligned
	// ratioFloor and ratioFull map the box/reference ratio onto evidence:
	// at or below 1.2 the text behaves like the rest, at 1.7 or more it
	// clearly does not. Evidence 50 (scoring.ELAEvidenceMin) is a ratio of
	// 1.45; the highest original line in the benchmark reached 1.34.
	ratioFloor = 1.2
	ratioFull  = 1.7
	// minRefBlocks is how many text-like blocks the reference needs.
	minRefBlocks = 4
	// maxFullPixels bounds the full-image fallback when the band around a
	// box holds too little other text.
	maxFullPixels = 6 << 20
)

// textQualities are the re-compression qualities compared; a pasted line
// can stand out at one and not the other.
var textQualities = []int{75, 90}

// TextEvidence returns, for each normalised [ymin, xmin, ymax, xmax] box
// (0-1000), how strongly the text inside it reacts to re-compression unlike
// the other text of the image, from 0 to 100, or -1 when it cannot be
// measured (no text in the box, too little other text to compare with).
func TextEvidence(src image.Image, boxes [][4]int) []int {
	out := make([]int, len(boxes))
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 64 || h < 64 {
		for i := range out {
			out[i] = -1
		}
		return out
	}
	for i, nb := range boxes {
		out[i] = boxEvidence(src, w, h, pixelRect(nb, w, h))
	}
	return out
}

func pixelRect(nb [4]int, w, h int) image.Rectangle {
	y0, x0, y1, x1 := nb[0], nb[1], nb[2], nb[3]
	return image.Rect(x0*w/1000, y0*h/1000, (x1*w+999)/1000, (y1*h+999)/1000).Intersect(image.Rect(0, 0, w, h))
}

// boxEvidence measures one box in a band of rows around it (the lines of a
// caption sit close together), or in the whole image when the band holds
// too little other text.
func boxEvidence(src image.Image, w, h int, box image.Rectangle) int {
	if box.Dx() < 8 || box.Dy() < 6 {
		return -1
	}
	bh := box.Dy()
	top := max(0, box.Min.Y-4*bh) / textBlock * textBlock
	bottom := min(h, box.Max.Y+4*bh)
	ratio, ok := bandRatio(src, w, image.Rect(0, top, w, bottom), box)
	if !ok && (top > 0 || bottom < h) && w*h <= maxFullPixels {
		ratio, ok = bandRatio(src, w, image.Rect(0, 0, w, h), box)
	}
	if !ok {
		return -1
	}
	ev := (ratio - ratioFloor) / (ratioFull - ratioFloor) * 100
	return int(min(max(ev, 0), 100) + 0.5)
}

// bandRatio re-compresses the band (grid-aligned, so every 8x8 block keeps
// its place) and returns the largest box/reference ratio over the
// qualities.
func bandRatio(src image.Image, w int, band, box image.Rectangle) (float64, bool) {
	b := src.Bounds()
	crop := image.NewRGBA(image.Rect(0, 0, band.Dx(), band.Dy()))
	draw.Draw(crop, crop.Bounds(), src, b.Min.Add(band.Min), draw.Src)
	lum, grad := luminance(crop)
	local := box.Sub(band.Min)

	refs := TextLikeBlocks(lum, grad, crop.Rect.Dx(), crop.Rect.Dy(), local)
	if len(refs) < minRefBlocks {
		return 0, false
	}
	// The box must hold lettering itself: photo texture would be compared
	// with text and always stand out.
	boxGrad := sumRect(grad, crop.Rect.Dx(), local)
	if boxGrad < float64(local.Dx()*local.Dy())*8 || !twoTone(lum, crop.Rect.Dx(), local, 0.6) {
		return 0, false
	}

	best := 0.0
	for _, q := range textQualities {
		ela, err := elaMap(crop, q)
		if err != nil {
			return 0, false
		}
		boxRatio := sumRect(ela, crop.Rect.Dx(), local) / boxGrad
		ratios := make([]float64, 0, len(refs))
		for _, r := range refs {
			ratios = append(ratios, sumRect(ela, crop.Rect.Dx(), r)/(sumRect(grad, crop.Rect.Dx(), r)+1e-6))
		}
		slices.Sort(ratios)
		ref := ratios[len(ratios)/2]
		if ref <= 0 {
			continue
		}
		best = max(best, boxRatio/ref)
	}
	return best, best > 0
}

// luminance returns the luma and the |dx|+|dy| edge strength of img.
func luminance(img *image.RGBA) ([]float32, []float32) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	lum := make([]float32, w*h)
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			r, g, b := row[4*x], row[4*x+1], row[4*x+2]
			lum[y*w+x] = 0.299*float32(r) + 0.587*float32(g) + 0.114*float32(b)
		}
	}
	grad := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var g float32
			if x > 0 {
				g += abs32(lum[y*w+x] - lum[y*w+x-1])
			}
			if y > 0 {
				g += abs32(lum[y*w+x] - lum[(y-1)*w+x])
			}
			grad[y*w+x] = g
		}
	}
	return lum, grad
}

// elaMap re-encodes img at quality q and returns the per-pixel mean absolute
// RGB difference.
func elaMap(img *image.RGBA, q int) ([]float32, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		return nil, err
	}
	re, err := jpeg.Decode(&buf)
	if err != nil {
		return nil, err
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), re, re.Bounds().Min, draw.Src)
	out := make([]float32, w*h)
	for y := 0; y < h; y++ {
		a, b := img.Pix[y*img.Stride:], rgba.Pix[y*rgba.Stride:]
		for x := 0; x < w; x++ {
			d := absDiff8(a[4*x], b[4*x]) + absDiff8(a[4*x+1], b[4*x+1]) + absDiff8(a[4*x+2], b[4*x+2])
			out[y*w+x] = float32(d) / 3
		}
	}
	return out, nil
}

// TextLikeBlocks finds 16x16 blocks that look like lettering: strong edges
// and a two-tone histogram (strokes on a plain background), unlike photo
// texture. Blocks touching exclude are left out.
func TextLikeBlocks(lum, grad []float32, w, h int, exclude image.Rectangle) []image.Rectangle {
	var out []image.Rectangle
	for y := 0; y+textBlock <= h; y += textBlock {
		for x := 0; x+textBlock <= w; x += textBlock {
			r := image.Rect(x, y, x+textBlock, y+textBlock)
			if r.Overlaps(exclude.Inset(-4)) {
				continue
			}
			if sumRect(grad, w, r) < 12*textBlock*textBlock {
				continue
			}
			if twoTone(lum, w, r, 0.75) {
				out = append(out, r)
			}
		}
	}
	return out
}

// twoTone reports whether the pixels in r split into two well-separated
// tones (strokes and background): the 10th and 90th luma percentiles are at
// least 80 apart and at least minNear of the pixels sit near one of them.
func twoTone(lum []float32, w int, r image.Rectangle, minNear float64) bool {
	vals := make([]float32, 0, r.Dx()*r.Dy())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		vals = append(vals, lum[y*w+r.Min.X:y*w+r.Max.X]...)
	}
	if len(vals) == 0 {
		return false
	}
	slices.Sort(vals)
	lo, hi := vals[len(vals)/10], vals[len(vals)*9/10]
	spread := hi - lo
	if spread < 80 {
		return false
	}
	near := 0
	for _, v := range vals {
		if v-lo < 0.2*spread || hi-v < 0.2*spread {
			near++
		}
	}
	return float64(near) >= minNear*float64(len(vals))
}

func sumRect(m []float32, w int, r image.Rectangle) float64 {
	var s float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for _, v := range m[y*w+r.Min.X : y*w+r.Max.X] {
			s += float64(v)
		}
	}
	return s
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func absDiff8(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
