package security

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders
	_ "image/png"
	"io"

	_ "golang.org/x/image/webp" // WebP is accepted by magic bytes; decoding needs this
)

// Upload errors. Each maps to a distinct API error code at the handler layer.
var (
	ErrTooLarge      = errors.New("upload too large")
	ErrUnsupported   = errors.New("unsupported media type")
	ErrTooManyPixels = errors.New("image dimensions exceed the limit")
	ErrAnimated      = errors.New("animated images are not supported")
)

// Limits from TECH_SPEC §7.2.
const (
	MaxDimension = 4096
	MaxPixels    = 16 << 20
)

// magicPrefixes are the only accepted file signatures. The client-supplied
// Content-Type is ignored entirely.
var magicPrefixes = []struct {
	name   string
	prefix []byte
}{
	{"jpeg", []byte{0xFF, 0xD8, 0xFF}},
	{"png", []byte{0x89, 0x50, 0x4E, 0x47}},
	{"webp", []byte{'R', 'I', 'F', 'F'}},
}

// ValidateUpload reads at most maxBytes+1 from r and returns the decoded image
// config plus the detected format. It never writes to disk.
//
// The returned bytes are the caller's to keep in memory only.
func ValidateUpload(r io.Reader, maxBytes int64) ([]byte, string, image.Config, error) {
	var cfg image.Config

	limited := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", cfg, fmt.Errorf("read upload: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", cfg, ErrTooLarge
	}
	if len(data) < 12 {
		return nil, "", cfg, ErrUnsupported
	}

	format := detectMagic(data)
	if format == "" {
		return nil, "", cfg, ErrUnsupported
	}

	// WebP needs the RIFF....WEBP shape, not just a RIFF prefix.
	if format == "webp" && !bytes.Equal(data[8:12], []byte("WEBP")) {
		return nil, "", cfg, ErrUnsupported
	}
	if isAnimated(data, format) {
		return nil, "", cfg, ErrAnimated
	}

	// DecodeConfig reads only the header, so a decompression bomb never gets
	// fully decoded. The standard library has no WebP decoder; its header is
	// read directly (WebP images are never decoded by AMBA, only passed on).
	if format == "webp" {
		w, h, ok := webpSize(data)
		if !ok {
			return nil, "", cfg, ErrUnsupported
		}
		cfg = image.Config{Width: w, Height: h}
	} else {
		var decodedFormat string
		cfg, decodedFormat, err = image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, "", cfg, ErrUnsupported
		}
		if decodedFormat != format {
			return nil, "", cfg, ErrUnsupported
		}
	}
	if cfg.Width > MaxDimension || cfg.Height > MaxDimension {
		return nil, "", cfg, ErrTooManyPixels
	}
	if cfg.Width*cfg.Height > MaxPixels {
		return nil, "", cfg, ErrTooManyPixels
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, "", cfg, ErrUnsupported
	}
	return data, format, cfg, nil
}

// webpSize reads the canvas size from the first chunk of a WebP file: a
// lossy VP8 frame header, a lossless VP8L header, or the extended VP8X
// header.
func webpSize(d []byte) (int, int, bool) {
	if len(d) < 30 || string(d[0:4]) != "RIFF" || string(d[8:12]) != "WEBP" {
		return 0, 0, false
	}
	switch string(d[12:16]) {
	case "VP8 ":
		// 3-byte frame tag, then the key frame start code 9d 01 2a.
		if d[23] != 0x9d || d[24] != 0x01 || d[25] != 0x2a {
			return 0, 0, false
		}
		w := int(binary.LittleEndian.Uint16(d[26:28]) & 0x3fff)
		h := int(binary.LittleEndian.Uint16(d[28:30]) & 0x3fff)
		return w, h, w > 0 && h > 0
	case "VP8L":
		if d[20] != 0x2f {
			return 0, 0, false
		}
		bits := binary.LittleEndian.Uint32(d[21:25])
		return int(bits&0x3fff) + 1, int(bits>>14&0x3fff) + 1, true
	case "VP8X":
		w := 1 + (int(d[24]) | int(d[25])<<8 | int(d[26])<<16)
		h := 1 + (int(d[27]) | int(d[28])<<8 | int(d[29])<<16)
		return w, h, true
	}
	return 0, 0, false
}

func detectMagic(data []byte) string {
	for _, m := range magicPrefixes {
		if bytes.HasPrefix(data, m.prefix) {
			return m.name
		}
	}
	return ""
}

// isAnimated checks the container markers for multi-frame images. Go's stdlib
// decoders would silently return the first frame otherwise.
func isAnimated(data []byte, format string) bool {
	switch format {
	case "png":
		// APNG declares an acTL chunk before the first IDAT.
		return bytes.Contains(data[:min(len(data), 4096)], []byte("acTL"))
	case "webp":
		return bytes.Contains(data[:min(len(data), 4096)], []byte("ANIM"))
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
