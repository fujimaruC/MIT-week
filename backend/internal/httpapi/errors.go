package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/amba/amba/backend/internal/models"
)

// apiError is an internal error carrying the HTTP status, stable code and
// Indonesian message. Only this type is ever turned into a response body.
type apiError struct {
	status    int
	code      string
	message   string
	retryable bool
}

func (e *apiError) Error() string { return e.code + ": " + e.message }

func newAPIError(status int, code, msg string, retryable bool) *apiError {
	return &apiError{status: status, code: code, message: msg, retryable: retryable}
}

// writeError renders the single error envelope. Nothing else is ever written
// on an error path: no stack traces, no internal detail, no upstream text.
func writeError(w http.ResponseWriter, err *apiError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(err.status)
	_ = json.NewEncoder(w).Encode(models.ErrorEnvelope{
		Error: models.ErrorBody{Code: err.code, Message: err.message, Retryable: err.retryable},
	})
}

// writeJSON writes a success payload.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("response encode failed") // never logs payload content
	}
}

// Common errors, kept in one place so messages stay consistent.
var (
	errInvalidURL       = newAPIError(http.StatusBadRequest, models.CodeInvalidURL, "Tautan tidak valid.", false)
	errBlockedTarget    = newAPIError(http.StatusBadRequest, models.CodeBlockedTarget, "Tautan ini mengarah ke alamat yang bukan situs publik, jadi ditolak demi keamanan.", false)
	errUnsupportedMedia = newAPIError(http.StatusUnsupportedMediaType, models.CodeUnsupportedMedia, "Jenis berkas ini belum didukung.", false)
	errPayloadTooLarge  = newAPIError(http.StatusRequestEntityTooLarge, models.CodePayloadTooLarge, "Berkas terlalu besar. Maksimal 10 MB.", false)
	errRateLimited      = newAPIError(http.StatusTooManyRequests, models.CodeRateLimited, "Terlalu banyak pemeriksaan. Coba lagi sebentar.", true)
	errUpstreamTimeout  = newAPIError(http.StatusGatewayTimeout, models.CodeUpstreamTimeout, "Pemeriksaan belum selesai. Coba lagi.", true)
	errInternal         = newAPIError(http.StatusInternalServerError, models.CodeInternal, "Terjadi kendala di sisi kami. Coba lagi.", true)
)
