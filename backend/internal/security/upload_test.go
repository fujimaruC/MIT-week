package security

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// encodeJPEG returns a valid JPEG of the given size.
func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), 100, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{10, 20, 30, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestValidateUploadAccepts(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		format string
	}{
		{"jpeg", encodeJPEG(t, 64, 64), "jpeg"},
		{"png", encodePNG(t, 64, 64), "png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, format, cfg, err := ValidateUpload(bytes.NewReader(tt.data), 10<<20)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if format != tt.format {
				t.Errorf("format = %q, want %q", format, tt.format)
			}
			if cfg.Width != 64 || cfg.Height != 64 {
				t.Errorf("dims = %dx%d, want 64x64", cfg.Width, cfg.Height)
			}
		})
	}
}

func TestValidateUploadRejects(t *testing.T) {
	pngData := encodePNG(t, 32, 32)
	webpHeader := append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
	apng := append([]byte{}, pngData...)
	// Insert an acTL chunk marker to simulate an animated PNG.
	apng = append(apng[:33], append([]byte("acTL"), apng[33:]...)...)

	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", []byte{}, ErrUnsupported},
		{"too short", []byte{0xFF, 0xD8}, ErrUnsupported},
		{"gif", []byte("GIF89a................"), ErrUnsupported},
		{"pdf", []byte("%PDF-1.7............"), ErrUnsupported},
		{"plain text", []byte("hello world, not an image"), ErrUnsupported},
		{"riff but not webp", []byte("RIFF\x10\x00\x00\x00WAVEfmt "), ErrUnsupported},
		{"truncated jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, ErrUnsupported},
		{"animated png", apng, ErrAnimated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := ValidateUpload(bytes.NewReader(tt.data), 10<<20)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
	_ = webpHeader
}

// webpFile builds a WebP container whose first chunk is typ with data.
func webpFile(typ string, data []byte, more ...string) []byte {
	body := append([]byte("WEBP"+typ), byte(len(data)), byte(len(data)>>8), byte(len(data)>>16), byte(len(data)>>24))
	body = append(body, data...)
	for _, m := range more {
		body = append(body, m...)
	}
	n := len(body)
	return append([]byte{'R', 'I', 'F', 'F', byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)}, body...)
}

// WebP has no decoder in the standard library; the header gives the size.
func TestValidateUploadReadsWebPHeaders(t *testing.T) {
	vp8x := webpFile("VP8X", []byte{0x10, 0, 0, 0, 0x7F, 0x02, 0x00, 0xDF, 0x01, 0x00})                 // 640x480, with ALPH flag
	lossy := webpFile("VP8 ", []byte{0x50, 0x2C, 0x00, 0x9D, 0x01, 0x2A, 0x2C, 0x01, 0xC8, 0x00, 0, 0}) // 300x200
	// Lossless: width-1 = 99, height-1 = 49 packed in 14-bit fields.
	bits := uint32(99) | uint32(49)<<14
	lossless := webpFile("VP8L", []byte{0x2F, byte(bits), byte(bits >> 8), byte(bits >> 16), byte(bits >> 24), 0, 0, 0, 0, 0})
	for name, tc := range map[string]struct {
		data []byte
		w, h int
	}{
		"extended": {vp8x, 640, 480},
		"lossy":    {lossy, 300, 200},
		"lossless": {lossless, 100, 50},
	} {
		t.Run(name, func(t *testing.T) {
			_, format, cfg, err := ValidateUpload(bytes.NewReader(tc.data), 10<<20)
			if err != nil || format != "webp" || cfg.Width != tc.w || cfg.Height != tc.h {
				t.Fatalf("got %s %dx%d %v, want webp %dx%d", format, cfg.Width, cfg.Height, err, tc.w, tc.h)
			}
		})
	}

	for name, tc := range map[string]struct {
		data []byte
		want error
	}{
		"bad start code": {webpFile("VP8 ", []byte{0x50, 0x2C, 0x00, 0x00, 0x00, 0x00, 0x2C, 0x01, 0xC8, 0x00, 0, 0}), ErrUnsupported},
		"bad signature":  {webpFile("VP8L", []byte{0x00, 1, 2, 3, 4, 0, 0, 0, 0, 0}), ErrUnsupported},
		"unknown chunk":  {webpFile("ABCD", make([]byte, 10)), ErrUnsupported},
		"too wide":       {webpFile("VP8X", []byte{0, 0, 0, 0, 0x87, 0x13, 0x00, 0x09, 0x00, 0x00}), ErrTooManyPixels}, // 5000x10
		"animated":       {webpFile("VP8X", []byte{0x02, 0, 0, 0, 0x7F, 0x02, 0x00, 0xDF, 0x01, 0x00}, "ANIM\x06\x00\x00\x00"), ErrAnimated},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := ValidateUpload(bytes.NewReader(tc.data), 10<<20); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateUploadSizeLimit(t *testing.T) {
	data := encodeJPEG(t, 64, 64)
	_, _, _, err := ValidateUpload(bytes.NewReader(data), int64(len(data)-1))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestValidateUploadDimensionLimit(t *testing.T) {
	// A 5000px-wide PNG is over MaxDimension even though it compresses small.
	data := encodePNG(t, 5000, 4)
	_, _, _, err := ValidateUpload(bytes.NewReader(data), 10<<20)
	if !errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("err = %v, want ErrTooManyPixels", err)
	}
}

func TestValidateUploadIgnoresClientMIME(t *testing.T) {
	// The signature decides, never a caller-supplied type string.
	data := encodePNG(t, 16, 16)
	_, format, _, err := ValidateUpload(bytes.NewReader(data), 10<<20)
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" {
		t.Fatalf("format = %q, want png", format)
	}
}
