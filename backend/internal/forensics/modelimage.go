package forensics

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
)

// ModelImage prepares an upload for the vision model: decodable images are
// re-encoded as JPEG (quality 85) and downscaled so the longest side is at
// most maxSide, which keeps the request small without moving the 0-1000
// normalised boxes the model reports. Formats the standard library cannot
// decode (WebP) are passed through unchanged with their own MIME type.
func ModelImage(encoded []byte, format string, maxSide int) ([]byte, string) {
	src, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return encoded, mimeFor(format)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return encoded, mimeFor(format)
	}
	if format == "jpeg" && w <= maxSide && h <= maxSide && len(encoded) <= 4<<20 {
		return encoded, "image/jpeg"
	}

	dst := src
	if w > maxSide || h > maxSide {
		scale := float64(maxSide) / float64(max(w, h))
		dw, dh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
		dst = downscale(src, dw, dh)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return encoded, mimeFor(format)
	}
	return buf.Bytes(), "image/jpeg"
}

// downscale averages a 2x2 sample grid per destination pixel: cheap, and
// smooth enough that text stays legible for OCR.
func downscale(src image.Image, dw, dh int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var r, g, bl, n uint32
			for _, oy := range [2]float64{0.25, 0.75} {
				for _, ox := range [2]float64{0.25, 0.75} {
					sx := b.Min.X + min(sw-1, int((float64(x)+ox)*float64(sw)/float64(dw)))
					sy := b.Min.Y + min(sh-1, int((float64(y)+oy)*float64(sh)/float64(dh)))
					cr, cg, cb, _ := src.At(sx, sy).RGBA()
					r, g, bl, n = r+cr, g+cg, bl+cb, n+1
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), 255})
		}
	}
	return out
}

func mimeFor(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
