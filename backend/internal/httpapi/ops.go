package httpapi

import (
	"context"
	"net/http"
	"time"
)

// opsStatus is the honest integration map. Every external dependency is
// either live, explicitly disabled, or a placeholder — never silently absent.
// It carries no key, URL, host or model output.
type opsStatus struct {
	Status       string            `json:"status"`
	Version      string            `json:"version"`
	UptimeS      int64             `json:"uptime_seconds"`
	Requests     int64             `json:"requests"`
	LLM          llmStatus         `json:"llm"`
	Retrieval    integrationStatus `json:"retrieval"`
	SafeBrowsing integrationStatus `json:"safe_browsing"`
	Pillars      map[string]string `json:"pillars"`
	Stats        map[string]any    `json:"stats"`
}

type llmStatus struct {
	Mode        string `json:"mode"`
	Model       string `json:"model"`
	Found       bool   `json:"model_found"`
	ProbeFailed bool   `json:"probe_failed"`
	Vision      bool   `json:"vision"`
	Reasoning   string `json:"reasoning_effort"`
	Placeholder bool   `json:"placeholder"`
}

type integrationStatus struct {
	Enabled     bool   `json:"enabled"`
	Configured  bool   `json:"configured"`
	Placeholder bool   `json:"placeholder"`
	Note        string `json:"note,omitempty"`
}

// getOpsStatus serves GET /api/v1/ops/status.
//
// The gateway's /models call is best-effort: a cold or briefly busy gateway
// would otherwise make a healthy server look misconfigured. The live audit
// path is the real health signal, so a failed probe is reported as "unknown"
// rather than "not found".
func (h *handlers) getOpsStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	client := h.pipeline.LLM
	reachable, listed := client.HealthCheck(ctx)
	probeFailed := !client.Mock && !reachable

	llmMode := "live"
	placeholder := false
	if client.Mock {
		llmMode = "mock"
		placeholder = true
	}

	searcher := h.pipeline.Searcher
	retrieval := integrationStatus{
		Enabled:    searcher != nil && searcher.Enabled(),
		Configured: searcher != nil && searcher.Enabled(),
		Note:       "Pencarian pemeriksa fakta dan berita, lalu disaring daftar sumber tepercaya.",
	}
	if !retrieval.Enabled {
		retrieval.Placeholder = true
		retrieval.Note = "Nonaktif: pemeriksaan fakta tidak berjalan."
	}

	safe := integrationStatus{
		Enabled:    h.pipeline.SafeBrowsing.Enabled(),
		Configured: h.pipeline.SafeBrowsing.Enabled(),
		Note:       "Google Safe Browsing v4.",
	}
	if !safe.Enabled {
		safe.Placeholder = true
		safe.Note = "Placeholder: GOOGLE_SAFE_BROWSING_KEY belum diisi, jadi reputasi tautan tidak diperiksa."
	}

	pillars := map[string]string{
		"ela":  "live",
		"ai":   llmMode,
		"fact": "live",
		"link": "live",
	}
	if !retrieval.Enabled {
		pillars["fact"] = "placeholder"
	}
	if llmMode == "mock" {
		pillars["ai"] = "placeholder"
	}
	if safe.Placeholder {
		pillars["link"] = "live+placeholder"
	}

	writeJSON(w, http.StatusOK, opsStatus{
		Status:   "ok",
		Version:  "1.0.0",
		UptimeS:  int64(time.Since(h.startedAt).Seconds()),
		Requests: h.requests.Load(),
		LLM: llmStatus{
			Mode:        llmMode,
			Model:       h.cfg.RouterModel,
			Found:       listed,
			ProbeFailed: probeFailed,
			Vision:      !client.Mock,
			Reasoning:   h.cfg.Reasoning,
			Placeholder: placeholder,
		},
		Retrieval:    retrieval,
		SafeBrowsing: safe,
		Pillars:      pillars,
		Stats: map[string]any{
			"pipeline": h.pipeline.Stats.Snapshot(),
			"llm":      client.Stats.Snapshot(),
		},
	})
}
