package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/scoring"
)

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// letters draws pseudo-lettering (bars in 14x22 cells) and returns its box.
func letters(img *image.RGBA, x, y, cells, seed int) image.Rectangle {
	ink := color.RGBA{20, 20, 20, 255}
	for c := 0; c < cells; c++ {
		cx := x + c*16
		for k := 0; k < 3; k++ {
			v := (seed*31 + c*17 + k*7) % 5
			if k%2 == 0 {
				fill(img, image.Rect(cx+v*2, y, cx+v*2+3, y+22), ink)
			} else {
				fill(img, image.Rect(cx, y+v*4, cx+12, y+v*4+3), ink)
			}
		}
	}
	return image.Rect(x, y, x+cells*16, y+22)
}

func reencode(t *testing.T, img image.Image, q int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeRGBA(t *testing.T, data []byte) *image.RGBA {
	t.Helper()
	src, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewRGBA(src.Bounds())
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			out.Set(x, y, src.At(x, y))
		}
	}
	return out
}

// retypedCaption is a circulated caption card whose last line was covered
// and retyped in an editor: the classic price hoax.
func retypedCaption(t *testing.T) ([]byte, [4]int) {
	const w, h = 480, 360
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := uint8((x*7 + y*13 + (x*y)%23) % 40)
			img.SetRGBA(x, y, color.RGBA{70 + n, 110 + n/2, 90 + n, 255})
		}
	}
	paper := color.RGBA{245, 245, 240, 255}
	fill(img, image.Rect(0, 200, w, h), paper)
	letters(img, 24, 216, 22, 1)
	letters(img, 24, 256, 26, 2)
	line := letters(img, 24, 300, 14, 3)
	circulated := decodeRGBA(t, reencode(t, decodeRGBA(t, reencode(t, img, 80)), 75))
	fill(circulated, line.Inset(-3), paper)
	retyped := letters(circulated, 24, 300, 14, 9)
	box := [4]int{retyped.Min.Y * 1000 / h, retyped.Min.X * 1000 / w, retyped.Max.Y * 1000 / h, retyped.Max.X * 1000 / w}
	return reencode(t, circulated, 95), box
}

// The model marks the retyped line; the measurement agrees, so the verdict
// says the image was edited.
func TestRetypedTextIsCorroborated(t *testing.T) {
	data, box := retypedCaption(t)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		inner := fmt.Sprintf(`{"claim":"","search_query":"","ai_score":5,"ai_indicators":[],"ai_watermark":"","text_anomalies":[{"bounding_box":[%d,%d,%d,%d],"extracted_text":"Rp 2.000","reason":"Huruf harga berbeda"},{"bounding_box":[50,50,150,300],"extracted_text":"","reason":"Area foto"}],"ocr_text":"Rp 2.000","tactics":[],"injection_suspected":false}`, box[0], box[1], box[2], box[3])
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": inner}}}})
	}))
	defer gateway.Close()
	p := newTestPipeline(false)
	p.LLM = llm.New(gateway.URL, "k", "m", false)

	res := p.Run(context.Background(), Request{InputType: models.InputFile, Image: data, ImageFormat: "jpeg"})
	ela := res.Pillars.ELA
	if len(ela.Anomalies) != 2 || ela.Anomalies[0].Evidence == nil || *ela.Anomalies[0].Evidence < TextHotspotMin {
		t.Fatalf("anomalies = %+v", ela.Anomalies)
	}
	if ela.Anomalies[1].Evidence != nil {
		t.Errorf("a box without lettering must stay unmeasured: %d", *ela.Anomalies[1].Evidence)
	}
	if ela.Score < TextHotspotMin {
		t.Fatalf("ELA score %d did not take the text evidence", ela.Score)
	}
	v := Verdict(res, false)
	if !slices.Contains(v.FloorsApplied, scoring.FloorELACorroborated) || v.Status != models.StatusTampered {
		t.Fatalf("verdict = %+v", v)
	}
	if v.ExplanationSimple != "Gambar ini terindikasi diedit. Jangan langsung percaya." {
		t.Errorf("lead sentence = %q", v.ExplanationSimple)
	}
}

// Without boxes from the model nothing is measured, and the old behaviour
// (global ELA only) stands.
func TestNoBoxesNoTextEvidence(t *testing.T) {
	data, _ := retypedCaption(t)
	res := newTestPipeline(true).Run(context.Background(), Request{InputType: models.InputFile, Image: data, ImageFormat: "jpeg"})
	for _, a := range res.Pillars.ELA.Anomalies {
		if a.Evidence != nil {
			t.Fatalf("evidence without a model box: %+v", a)
		}
	}
}
