// Package aidetect asks a dedicated AI-image detector how likely an image is
// to be AI-generated (PRD "Jejak AI"). It is optional: without credentials it
// is disabled, and the AI pillar relies on the file's provenance metadata and
// the vision model alone.
//
// The image is sent to the detector service and nothing else: no file name,
// no text, no user data. Operators who enable it must say so in the privacy
// notice (frontend /tentang).
package aidetect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

// DefaultSightengineEndpoint is Sightengine's image check endpoint; the
// "genai" model returns the probability that an image is AI-generated.
const DefaultSightengineEndpoint = "https://api.sightengine.com/1.0/check.json"

// ErrFailed means the detector did not answer usefully. The caller treats it
// as "not checked", never as "not AI".
var ErrFailed = errors.New("ai detector call failed")

// Sightengine is the client for Sightengine's AI-generated image detection.
type Sightengine struct {
	User     string
	Secret   string
	Endpoint string
	HTTP     *http.Client
}

// NewSightengine builds a client; empty credentials yield a disabled client.
func NewSightengine(user, secret string) *Sightengine {
	return &Sightengine{
		User:     user,
		Secret:   secret,
		Endpoint: DefaultSightengineEndpoint,
		HTTP:     &http.Client{Timeout: 5 * time.Second},
	}
}

// Enabled reports whether checks will be attempted.
func (s *Sightengine) Enabled() bool { return s != nil && s.User != "" && s.Secret != "" }

// Name is the detector's name as shown in the Dossier.
func (s *Sightengine) Name() string { return "sightengine" }

// MaxImageBytes caps what is sent to the detector. Uploads are already capped
// at the entry (security.ValidateUpload), but link preview images arrive by
// another road, so the detector checks again instead of trusting its caller.
const MaxImageBytes = 10 << 20

// sniffImage is the image's real type, read from its bytes; "" for anything
// that is not a JPEG, PNG, WebP or GIF. A client's Content-Type is never
// trusted: a text file can call itself image/jpeg.
func sniffImage(image []byte) string {
	switch t := http.DetectContentType(image); t {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return t
	}
	return ""
}

// Score returns how likely the image is to be AI-generated, 0 to 100. The
// caller's MIME type is ignored: the type is sniffed from the bytes, and an
// empty, oversized or non-image payload fails before any network call.
func (s *Sightengine) Score(ctx context.Context, image []byte, _ string) (int, error) {
	if !s.Enabled() || len(image) == 0 || len(image) > MaxImageBytes {
		return 0, ErrFailed
	}
	mime := sniffImage(image)
	if mime == "" {
		slog.Warn("ai detector call skipped", "reason", "not_an_image", "status", 0)
		return 0, ErrFailed
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range map[string]string{"models": "genai", "api_user": s.User, "api_secret": s.Secret} {
		if err := w.WriteField(k, v); err != nil {
			return 0, ErrFailed
		}
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="media"; filename="image"`)
	h.Set("Content-Type", mime)
	part, err := w.CreatePart(h)
	if err != nil {
		return 0, ErrFailed
	}
	if _, err := part.Write(image); err != nil {
		return 0, ErrFailed
	}
	if err := w.Close(); err != nil {
		return 0, ErrFailed
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint, &body)
	if err != nil {
		return 0, ErrFailed
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := s.HTTP.Do(req)
	if err != nil {
		reason := "transport"
		if ctx.Err() != nil {
			reason = "timeout"
		}
		slog.Warn("ai detector call failed", "reason", reason, "status", 0)
		return 0, ErrFailed
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// 401/403: bad credentials; 429: monthly or per-minute quota.
		slog.Warn("ai detector call failed", "reason", "http_status", "status", resp.StatusCode)
		return 0, ErrFailed
	}
	var out struct {
		Status string `json:"status"`
		Type   struct {
			AIGenerated *float64 `json:"ai_generated"`
		} `json:"type"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil ||
		out.Status != "success" || out.Type.AIGenerated == nil {
		slog.Warn("ai detector call failed", "reason", "unreadable_response", "status", resp.StatusCode)
		return 0, ErrFailed
	}
	p := *out.Type.AIGenerated
	if math.IsNaN(p) {
		return 0, ErrFailed
	}
	return int(math.Round(min(max(p, 0), 1) * 100)), nil
}
