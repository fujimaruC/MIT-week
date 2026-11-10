// Package forensics computes server-side Error Level Analysis.
//
// The method re-encodes the image as JPEG and measures how much each pixel
// changes. Regions that were pasted or edited usually change differently from
// their surroundings. It is an INDICATION, never proof: resizing, cropping,
// screenshots and ordinary compression all create differences too.
package forensics

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // register the PNG decoder
	"math"
)

// Limits and constants from TECH_SPEC §3.4.
const (
	MaxSide     = 4096
	MaxPixels   = 16 << 20
	GridSize    = 16
	JPEGQuality = 90
	// varianceScale maps the mean absolute difference onto a 0-100 score.
	varianceScale = 3.5
)

// ErrTooLarge is returned when the image exceeds the decode limits.
var ErrTooLarge = errors.New("image dimensions exceed limits")

// Result is the ELA outcome.
type Result struct {
	Score       int
	Reliability float64
	Note        string
	// Grid holds GridSize x GridSize cell means, row-major, used for hotspot
	// corroboration against LLM anomaly boxes.
	Grid   [GridSize][GridSize]float64
	Width  int
	Height int
	Format string
}

// Analyze decodes img, re-encodes it as JPEG at quality 90, and measures the
// per-pixel difference. The caller supplies the original encoded bytes so the
// decoder format can drive reliability.
func Analyze(encoded []byte) (Result, error) {
	src, format, err := Decode(encoded)
	if err != nil {
		return Result{}, err
	}
	return AnalyzeImage(src, format)
}

// Decode checks the dimensions from the header first, so an oversized image
// is never decoded, then decodes it.
func Decode(encoded []byte) (image.Image, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(encoded))
	if err != nil {
		return nil, "", err
	}
	if cfg.Width > MaxSide || cfg.Height > MaxSide || cfg.Width*cfg.Height > MaxPixels {
		return nil, "", ErrTooLarge
	}
	src, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return nil, "", err
	}
	return src, format, nil
}

// AnalyzeImage is Analyze on an image Decode already returned.
func AnalyzeImage(src image.Image, format string) (Result, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		return Result{}, err
	}
	reencoded, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return Result{}, err
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return Result{}, ErrTooLarge
	}

	var total float64
	cellSum := [GridSize][GridSize]float64{}
	cellCount := [GridSize][GridSize]int{}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			orig := color.RGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.RGBA)
			enc := color.RGBAModel.Convert(reencoded.At(reencoded.Bounds().Min.X+x, reencoded.Bounds().Min.Y+y)).(color.RGBA)

			diff := math.Abs(float64(orig.R)-float64(enc.R)) +
				math.Abs(float64(orig.G)-float64(enc.G)) +
				math.Abs(float64(orig.B)-float64(enc.B))
			total += diff

			gx := x * GridSize / w
			gy := y * GridSize / h
			if gx >= GridSize {
				gx = GridSize - 1
			}
			if gy >= GridSize {
				gy = GridSize - 1
			}
			cellSum[gy][gx] += diff
			cellCount[gy][gx]++
		}
	}

	mean := total / float64(w*h*3)
	score := int(math.Floor(mean*varianceScale + 0.5))
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	var grid [GridSize][GridSize]float64
	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			if cellCount[y][x] > 0 {
				grid[y][x] = cellSum[y][x] / float64(cellCount[y][x]*3)
			}
		}
	}

	res := Result{Score: score, Grid: grid, Width: w, Height: h, Format: format}
	switch format {
	case "jpeg":
		res.Reliability = 1.0
	case "png", "webp":
		res.Reliability = 0.5
		res.Note = "ELA kurang andal pada PNG/tangkapan layar"
	default:
		res.Reliability = 0.5
	}
	if w < 64 || h < 64 {
		res.Reliability = 0
		res.Note = "Gambar terlalu kecil untuk diperiksa"
	}
	return res, nil
}

// Hotspots returns the grid cells above a multiple of the mean cell value,
// expressed as normalized [ymin, xmin, ymax, xmax] boxes on a 0-1000 scale.
func (r Result) Hotspots(factor float64) [][4]int {
	var mean, count float64
	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			mean += r.Grid[y][x]
			count++
		}
	}
	if count == 0 {
		return nil
	}
	mean /= count
	if mean <= 0 {
		return nil
	}
	threshold := mean * factor

	cell := 1000 / GridSize
	var out [][4]int
	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			if r.Grid[y][x] >= threshold {
				out = append(out, [4]int{y * cell, x * cell, (y + 1) * cell, (x + 1) * cell})
			}
		}
	}
	return out
}
