package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/pipeline"
	"github.com/amba/amba/backend/internal/security"
	"github.com/amba/amba/backend/internal/video"
)

// auditRequest is the JSON body for the url route.
type auditRequest struct {
	URL string `json:"url"`
}

// auditURL handles POST /audit/url.
func (h *handlers) auditURL(w http.ResponseWriter, r *http.Request) {
	var body auditRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		writeError(w, errInvalidURL)
		return
	}
	raw, uerr := security.ValidateInputURL(body.URL)
	if uerr != nil {
		writeError(w, newAPIError(http.StatusBadRequest, models.CodeInvalidURL, uerr.Message, false))
		return
	}
	if _, err := security.ValidateFetchURL(raw); err != nil {
		// Generic message: never reveal which rule refused the target.
		writeError(w, errBlockedTarget)
		return
	}
	h.runAudit(w, r, pipeline.Request{InputType: models.InputURL, URL: raw})
}

// auditFile handles POST /audit/file.
func (h *handlers) auditFile(w http.ResponseWriter, r *http.Request) {
	maxBytes := h.cfg.MaxUploadMB << 20
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, errPayloadTooLarge)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, errUnsupportedMedia)
		return
	}
	defer func() { _ = file.Close() }()

	data, format, _, err := security.ValidateUpload(file, maxBytes)
	switch {
	case errors.Is(err, security.ErrTooLarge):
		writeError(w, errPayloadTooLarge)
		return
	case errors.Is(err, security.ErrUnsupported), errors.Is(err, security.ErrAnimated):
		writeError(w, errUnsupportedMedia)
		return
	case errors.Is(err, security.ErrTooManyPixels):
		writeError(w, errPayloadTooLarge)
		return
	case err != nil:
		writeError(w, errUnsupportedMedia)
		return
	}

	// The filename is never stored or logged; only the format is used.
	_ = header
	ocr := r.FormValue("ocr_text")
	if len(ocr) > 5000 {
		ocr = truncateRunes(ocr, 5000)
	}
	h.runAudit(w, r, pipeline.Request{
		InputType:   models.InputFile,
		Image:       data,
		ImageFormat: format,
		OCRText:     ocr,
	})
}

// runAudit executes the pipeline and renders either SSE or a single JSON body.
func (h *handlers) runAudit(w http.ResponseWriter, r *http.Request, req pipeline.Request) {
	h.requests.Add(1)
	if !h.limiter.AcquireInFlight() {
		h.metrics.Inc("amba_rate_limited_total", "reason", "in_flight")
		w.Header().Set("Retry-After", "5")
		writeError(w, errRateLimited)
		return
	}
	defer h.limiter.ReleaseInFlight()

	// A social video link is downloaded and analysed frame by frame: it gets
	// its own, longer budget.
	timeout := h.cfg.HardTimeout
	if req.InputType == models.InputURL && h.pipeline.Video != nil && h.pipeline.Video.Enabled() {
		if _, ok := video.Platform(req.URL); ok && h.cfg.VideoTimeout > timeout {
			timeout = h.cfg.VideoTimeout
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	started := time.Now()

	stream := strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	var dossier models.Dossier
	degraded := false
	if stream {
		var ok bool
		dossier, degraded, ok = h.streamAudit(ctx, w, req)
		if !ok {
			return
		}
	} else {
		res := h.pipeline.Run(ctx, req)
		degraded = res.Degraded
		dossier = h.buildDossier(res, req)
		writeJSON(w, http.StatusOK, dossier)
	}
	h.metrics.Inc("amba_audits_total", "input_type", string(req.InputType), "status", string(dossier.Verdict.Status))
	// Codes only, never content: how often each rule fires in production is
	// what tells a false alarm from a useful rule.
	for _, floor := range dossier.Verdict.FloorsApplied {
		h.metrics.Inc("amba_floors_total", "floor", floor)
	}
	for _, reason := range dossier.Pillars.Link.Reasons {
		h.metrics.Inc("amba_link_reasons_total", "reason", reason.Code)
	}
	if degraded {
		h.metrics.Inc("amba_audits_degraded_total")
	}
	h.metrics.Observe("amba_audit_duration_seconds", time.Since(started))
}

// streamAudit emits stage -> partial -> verdict -> done (TECH_SPEC §2). Each
// pillar's events are sent the moment that pillar starts and finishes, so a
// fast pillar (the link check) reaches the user before the slow model call.
// A ": ping" comment every 10 s keeps proxies from closing a quiet stream.
func (h *handlers) streamAudit(ctx context.Context, w http.ResponseWriter, req pipeline.Request) (dossier models.Dossier, degraded, ok bool) {
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		writeError(w, errInternal)
		return models.Dossier{}, false, false
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	var mu sync.Mutex
	closed := false
	writeRaw := func(chunk string) bool {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return false
		}
		if _, err := io.WriteString(w, chunk); err != nil {
			closed = true
			return false
		}
		flusher.Flush()
		return true
	}
	send := func(event string, payload any) bool {
		// Stop only when the client left. The hard timeout is not a reason to
		// withhold the verdict: it is built from whatever finished.
		if errors.Is(ctx.Err(), context.Canceled) {
			return false
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		return writeRaw(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
	}

	pingDone := make(chan struct{})
	go func() {
		t := time.NewTicker(h.heartbeat())
		defer t.Stop()
		for {
			select {
			case <-pingDone:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				writeRaw(": ping\n\n")
			}
		}
	}()

	res := h.pipeline.RunStream(ctx, req, func(e pipeline.Event) {
		if !send("stage", map[string]string{"pillar": e.Pillar, "status": e.Status}) {
			return
		}
		if e.Status == "done" && e.Data != nil {
			send("partial", map[string]any{"pillar": e.Pillar, "data": normalizePillar(e.Data)})
		}
	})
	close(pingDone)

	dossier = h.buildDossier(res, req)
	degraded = res.Degraded
	if !send("verdict", dossier) {
		return dossier, degraded, true
	}
	send("done", struct{}{})
	mu.Lock()
	closed = true
	mu.Unlock()
	return dossier, degraded, true
}

func (h *handlers) heartbeat() time.Duration {
	if h.cfg.HardTimeout > 0 && h.cfg.HardTimeout < 10*time.Second {
		return h.cfg.HardTimeout / 2
	}
	return 10 * time.Second
}

// normalizePillar makes a streamed pillar honour the same "arrays, never
// null" rule as the final Dossier.
func normalizePillar(data any) any {
	switch p := data.(type) {
	case models.ELAPillar:
		p.Anomalies = nonNilAnomalies(p.Anomalies)
		return p
	case models.AIPillar:
		p.Indicators = nonNilStrings(p.Indicators)
		if p.Provenance.Kind == "" {
			p.Provenance.Kind = models.ProvenanceNone
		}
		return p
	case models.FactPillar:
		p.Sources = nonNilSources(p.Sources)
		if p.Status == "" {
			p.Status = models.FactUnverified
		}
		return p
	case models.LinkPillar:
		p.RedirectChain = nonNilStrings(p.RedirectChain)
		p.Reasons = nonNilLinkReasons(p.Reasons)
		return p
	}
	return data
}

// buildDossier assembles the contract object.
func (h *handlers) buildDossier(res pipeline.Result, req pipeline.Request) models.Dossier {
	return BuildDossier(h.signingKey, res, req)
}

// BuildDossier assembles and signs the contract object for a pipeline
// result. The Telegram bot uses it too, so both entry points share one
// verdict path.
func BuildDossier(key []byte, res pipeline.Result, req pipeline.Request) models.Dossier {
	verdict := pipeline.Verdict(res, false)
	d := models.Dossier{
		SchemaVersion:   2,
		ID:              NewID(),
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		InputType:       req.InputType,
		SourceReference: sourceReference(req),
		// Every audit ends in a verdict; a lookup that failed along the way is not
		// something the reader can act on, so the wire flag stays false (res.Degraded
		// still keeps the result out of the cache and feeds the metrics).
		Degraded:       false,
		IsDemo:         false,
		Verdict:        verdict,
		Pillars:        res.Pillars,
		Tactics:        res.Tactics,
		SourceMetadata: res.Metadata,
		OCRText:        res.OCRText,
		Video:          res.Video,
	}
	// The wire contract says these are arrays. Go marshals a nil slice as
	// null, which every strict client rejects; normalise before signing.
	if d.Tactics == nil {
		d.Tactics = []models.Tactic{}
	}
	d.Pillars.ELA.Anomalies = nonNilAnomalies(d.Pillars.ELA.Anomalies)
	d.Pillars.AI.Indicators = nonNilStrings(d.Pillars.AI.Indicators)
	if d.Pillars.AI.Provenance.Kind == "" {
		d.Pillars.AI.Provenance.Kind = models.ProvenanceNone
	}
	d.Pillars.Fact.Sources = nonNilSources(d.Pillars.Fact.Sources)
	d.Pillars.Link.RedirectChain = nonNilStrings(d.Pillars.Link.RedirectChain)
	d.Pillars.Link.Reasons = nonNilLinkReasons(d.Pillars.Link.Reasons)
	if d.Pillars.Fact.Status == "" {
		d.Pillars.Fact.Status = models.FactUnverified
	}
	d.Verdict.Basis = nonNilStrings(d.Verdict.Basis)
	d.Verdict.NotChecked = nonNilStrings(d.Verdict.NotChecked)
	d.Verdict.FloorsApplied = nonNilStrings(d.Verdict.FloorsApplied)
	d.Verdict.ShortTitle = shortTitle(verdict.Status)
	if d.Verdict.Explanation == "" {
		d.Verdict.Explanation = explanation(verdict, res)
	}
	if token, err := Sign(key, d); err == nil {
		d.ShareToken = token
	}
	return d
}

func sourceReference(req pipeline.Request) string {
	var ref string
	switch req.InputType {
	case models.InputURL:
		ref = req.URL
	default:
		ref = "Gambar yang diunggah"
	}
	return truncateRunes(ref, 200)
}

func shortTitle(s models.Status) string {
	switch s {
	case models.StatusTampered:
		return "Terindikasi hoaks atau manipulasi"
	case models.StatusSuspicious:
		return "Perlu diperiksa lebih lanjut"
	default: // SAFE, and INSUFFICIENT in old payloads
		return "Tidak ditemukan tanda bahaya"
	}
}

// pillarNames is the plain-language name of each pillar code.
var pillarNames = map[string]string{
	"ela":           "teks ditimpa (ELA)",
	"ai":            "jejak AI",
	"fact":          "cek fakta",
	"link":          "keamanan tautan",
	"llm_untrusted": "catatan: jawaban AI tidak dipakai sebagai bukti",
}

func names(codes []string) string {
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if n, ok := pillarNames[c]; ok {
			out = append(out, n)
		} else {
			out = append(out, c)
		}
	}
	return strings.Join(out, ", ")
}

// explanation is the Indonesian verdict note: what was checked and what was
// found. It never says that something "cannot be checked": the strength of the
// evidence is described, not the gaps (Sir, 10 Nov).
func explanation(v models.Verdict, res pipeline.Result) string {
	videoAnalysed := res.Video != nil && res.Video.Status == models.VideoOK
	var parts []string
	if md := res.Metadata; md != nil {
		switch md.Status {
		case models.MetadataBlocked:
			if !videoAnalysed {
				parts = append(parts, "Platform tidak mengizinkan pembacaan otomatis, jadi pemeriksaan memakai tautan, keterangan, dan pencarian web. Unggah tangkapan layar untuk hasil lebih rinci.")
			}
		case models.MetadataUnavailable:
			parts = append(parts, "Halaman tautan tidak menampilkan isi, jadi pemeriksaan memakai alamat tautannya.")
		}
	}
	basis := make([]string, 0, len(v.Basis))
	for _, b := range v.Basis {
		if b != "llm_untrusted" {
			basis = append(basis, b)
		}
	}
	if len(basis) > 0 {
		parts = append(parts, fmt.Sprintf("Diperiksa: %s.", names(basis)))
	}
	if n := pipeline.VideoNote(res); n != "" {
		parts = append(parts, n)
	}
	if n := pipeline.FactNote(res); n != "" {
		parts = append(parts, n)
	}
	parts = append(parts, "Hasil ini indikatif, bukan bukti pasti.")
	return truncateRunes(strings.Join(parts, " "), 400)
}

// The helpers below exist only so JSON never contains null where the contract
// promises an array. They are deliberately tiny: allocating a zero-length
// slice is cheaper than teaching every client to tolerate null.
func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func nonNilAnomalies(v []models.Anomaly) []models.Anomaly {
	if v == nil {
		return []models.Anomaly{}
	}
	return v
}

func nonNilSources(v []models.Source) []models.Source {
	if v == nil {
		return []models.Source{}
	}
	return v
}

func nonNilLinkReasons(v []models.LinkReason) []models.LinkReason {
	if v == nil {
		return []models.LinkReason{}
	}
	return v
}
