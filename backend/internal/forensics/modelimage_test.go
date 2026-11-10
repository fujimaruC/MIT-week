package forensics

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestModelImageDownscalesAndReencodes(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 3000; x++ {
			src.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	out, mime := ModelImage(buf.Bytes(), "png", 1500)
	if mime != "image/jpeg" {
		t.Fatalf("mime = %s", mime)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1500 || b.Dy() != 500 {
		t.Fatalf("size = %v, want 1500x500 (aspect kept)", b)
	}
}

func TestModelImagePassesThroughSmallJPEGAndUnknownFormats(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 80))
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, src, nil)
	out, mime := ModelImage(buf.Bytes(), "jpeg", 1500)
	if mime != "image/jpeg" || !bytes.Equal(out, buf.Bytes()) {
		t.Fatal("a small JPEG should be sent as is")
	}
	webp := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	out, mime = ModelImage(webp, "webp", 1500)
	if mime != "image/webp" || !bytes.Equal(out, webp) {
		t.Fatalf("undecodable format must pass through, got %s", mime)
	}
}
