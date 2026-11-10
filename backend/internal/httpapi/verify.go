package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/amba/amba/backend/internal/token"
)

// verify handles POST /verify. The token arrives in the body, never the query,
// so it never reaches server logs.
func (h *handlers) verify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil || body.Token == "" {
		h.metrics.Inc("amba_verifications_total", "result", "invalid")
		writeJSON(w, http.StatusOK, map[string]any{"valid": false})
		return
	}

	payload, err := token.Verify(h.signingKey, body.Token)
	if err != nil {
		h.metrics.Inc("amba_verifications_total", "result", "invalid")
		writeJSON(w, http.StatusOK, map[string]any{"valid": false})
		return
	}

	h.metrics.Inc("amba_verifications_total", "result", "valid")
	writeJSON(w, http.StatusOK, map[string]any{
		"valid": true,
		"payload": map[string]any{
			"id":     payload.ID,
			"ts":     payload.TS,
			"status": payload.Status,
			"score":  payload.Score,
			"title":  payload.Title,
			"claim":  payload.Claim,
			"srcs":   payload.Srcs,
		},
	})
}
