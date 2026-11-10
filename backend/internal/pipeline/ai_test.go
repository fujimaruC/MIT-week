package pipeline

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/provenance"
	"github.com/amba/amba/backend/internal/scoring"
)

func TestBuildAIPillar(t *testing.T) {
	model := func(score int, watermark string, indicators ...string) llm.Findings {
		return llm.Findings{AIScore: score, AIWatermark: watermark, AIIndicators: indicators}
	}
	cases := []struct {
		name      string
		ev        aiEvidence
		wantOK    bool
		score     int
		rel       float64
		kind      models.ProvenanceKind
		source    string
		generator string
		detector  bool
		indicator string
	}{
		{
			name:   "visible clues only",
			ev:     aiEvidence{modelOK: true, findings: model(72, "", "Jari tangan kanan berjumlah enam")},
			wantOK: true, score: 72, rel: aiRelModel, kind: models.ProvenanceNone, indicator: "Jari tangan kanan berjumlah enam",
		},
		{
			name:   "a detector outweighs the model",
			ev:     aiEvidence{modelOK: true, findings: model(40, ""), det: detection{name: "sightengine", score: 96, ok: true}},
			wantOK: true, score: 79, rel: aiRelDetector, kind: models.ProvenanceNone, detector: true,
		},
		{
			name:   "a detector alone when the model failed",
			ev:     aiEvidence{det: detection{name: "sightengine", score: 12, ok: true}},
			wantOK: true, score: 12, rel: aiRelDetector, kind: models.ProvenanceNone, detector: true,
		},
		{
			name:   "Content Credentials declaring AI generation",
			ev:     aiEvidence{prov: provenance.Result{Kind: provenance.AIGenerated, Source: "c2pa", Generator: "OpenAI", C2PA: true}, modelOK: true, findings: model(20, "")},
			wantOK: true, score: 95, rel: aiRelDeclared, kind: models.ProvenanceAIGenerated, source: "c2pa", generator: "OpenAI",
		},
		{
			name:   "metadata alone decides when the model failed",
			ev:     aiEvidence{prov: provenance.Result{Kind: provenance.AIGenerated, Source: "png_text", Generator: "ComfyUI"}},
			wantOK: true, score: 95, rel: aiRelDeclared, kind: models.ProvenanceAIGenerated, source: "png_text", generator: "ComfyUI",
		},
		{
			name:   "an AI edit",
			ev:     aiEvidence{prov: provenance.Result{Kind: provenance.AIEdited, Source: "xmp"}, modelOK: true, findings: model(10, "")},
			wantOK: true, score: 80, rel: aiRelDeclared, kind: models.ProvenanceAIEdited, source: "xmp",
		},
		{
			name:   "a visible AI label the text confirms",
			ev:     aiEvidence{modelOK: true, findings: model(60, "Imagined with AI"), ocr: "BANJIR BESAR\nImagined with AI"},
			wantOK: true, score: 90, rel: aiRelLabel, kind: models.ProvenanceAIGenerated, source: "watermark", generator: "Meta",
		},
		{
			name:   "a label the text does not confirm stays a clue",
			ev:     aiEvidence{modelOK: true, findings: model(75, "Gemini"), ocr: "BANJIR BESAR"},
			wantOK: true, score: 75, rel: aiRelModel, kind: models.ProvenanceNone, indicator: "Tampak tanda air AI: Gemini",
		},
		{
			name:   "an article about AI is not a label",
			ev:     aiEvidence{modelOK: true, findings: model(5, ""), ocr: "Meta AI generated images flood social media"},
			wantOK: true, score: 5, rel: aiRelModel, kind: models.ProvenanceNone,
		},
		{
			name:   "camera metadata caps a visual judgement",
			ev:     aiEvidence{prov: provenance.Result{Kind: provenance.Camera, Source: "exif", Camera: "Canon EOS 80D"}, modelOK: true, findings: model(85, "")},
			wantOK: true, score: cameraCap, rel: aiRelModel, kind: models.ProvenanceCamera, source: "exif",
		},
		{
			name:   "nothing could judge the image",
			ev:     aiEvidence{prov: provenance.Result{Kind: provenance.Camera, Camera: "Canon"}},
			wantOK: false, kind: models.ProvenanceCamera,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := buildAIPillar(tc.ev)
			if ok != tc.wantOK || p.Available != tc.wantOK {
				t.Fatalf("ok=%v available=%v, want %v", ok, p.Available, tc.wantOK)
			}
			if p.Provenance.Kind != tc.kind {
				t.Errorf("kind = %s, want %s", p.Provenance.Kind, tc.kind)
			}
			if !ok {
				return
			}
			if p.Score != tc.score || p.Reliability != tc.rel {
				t.Errorf("score %d rel %.1f, want %d %.1f", p.Score, p.Reliability, tc.score, tc.rel)
			}
			if p.Provenance.Source != tc.source || p.Provenance.Generator != tc.generator {
				t.Errorf("provenance = %+v", p.Provenance)
			}
			if (p.Detector != nil) != tc.detector {
				t.Errorf("detector = %+v", p.Detector)
			}
			if tc.indicator != "" && !slices.Contains(p.Indicators, tc.indicator) {
				t.Errorf("indicators = %q, want %q", p.Indicators, tc.indicator)
			}
			if p.Caveat == "" || p.Indicators == nil {
				t.Errorf("caveat and indicators must be set: %+v", p)
			}
		})
	}
}

func TestBuildAIPillarBoundsIndicators(t *testing.T) {
	f := llm.Findings{AIScore: 90, AIWatermark: "Grok", AIIndicators: []string{"a", "b", "c", "d", "e"}}
	p, _ := buildAIPillar(aiEvidence{modelOK: true, findings: f})
	if len(p.Indicators) != maxAIIndicators {
		t.Fatalf("indicators = %q", p.Indicators)
	}
}

// withC2PA inserts a Content Credentials segment declaring AI generation
// into a JPEG, as ChatGPT or Firefly write it.
func withC2PA(jpeg []byte) []byte {
	payload := []byte("JP\x02\x11\x00\x00\x00\x01\x00\x00\x00\x40jumb\x00\x00\x00\x28jumdc2pa claim_generator ChatGPT c2pa.created digitalSourceType http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia")
	n := len(payload) + 2
	seg := append([]byte{0xFF, 0xEB, byte(n >> 8), byte(n)}, payload...)
	out := append([]byte{}, jpeg[:2]...)
	out = append(out, seg...)
	return append(out, jpeg[2:]...)
}

func TestUploadDeclaringAIIsFlagged(t *testing.T) {
	for _, mock := range []bool{true, false} { // false: the model is down
		res := newTestPipeline(mock).Run(context.Background(), Request{InputType: models.InputFile, Image: withC2PA(testJPEG(t, 40)), ImageFormat: "jpeg"})
		ai := res.Pillars.AI
		if !ai.Available || ai.Score < 95 || ai.Provenance.Kind != models.ProvenanceAIGenerated || ai.Provenance.Source != "c2pa" || ai.Provenance.Generator != "OpenAI" {
			t.Fatalf("mock=%v ai pillar = %+v", mock, ai)
		}
		v := Verdict(res, false)
		if v.Status != models.StatusTampered || v.Score == nil || *v.Score < 85 || !slices.Contains(v.FloorsApplied, scoring.FloorAIProvenance) {
			t.Fatalf("mock=%v verdict = %+v", mock, v)
		}
		if v.ExplanationSimple != "Ada tanda jelas gambar ini dibuat dengan AI. Jangan anggap sebagai foto asli." {
			t.Errorf("lead sentence = %q", v.ExplanationSimple)
		}
	}
}

func TestOrdinaryUploadHasNoProvenance(t *testing.T) {
	res := newTestPipeline(true).Run(context.Background(), imageRequest(t, 41, ""))
	if p := res.Pillars.AI.Provenance; p.Kind != models.ProvenanceNone || p.Source != "" || p.C2PA {
		t.Fatalf("provenance = %+v", p)
	}
	if res.Pillars.AI.Reliability != aiRelModel || res.Pillars.AI.Detector != nil {
		t.Fatalf("ai = %+v", res.Pillars.AI)
	}
}

type fakeDetector struct {
	score int
	err   error
	mu    sync.Mutex
	mimes []string
}

func (d *fakeDetector) Enabled() bool { return true }
func (d *fakeDetector) Name() string  { return "fake" }
func (d *fakeDetector) Score(_ context.Context, image []byte, mime string) (int, error) {
	d.mu.Lock()
	d.mimes = append(d.mimes, mime)
	d.mu.Unlock()
	return d.score, d.err
}

func TestDetectorJoinsTheAIPillar(t *testing.T) {
	p := newTestPipeline(true)
	det := &fakeDetector{score: 97}
	p.Detector = det
	res := p.Run(context.Background(), imageRequest(t, 42, ""))
	ai := res.Pillars.AI
	if ai.Detector == nil || ai.Detector.Name != "fake" || ai.Detector.Score != 97 || ai.Reliability != aiRelDetector {
		t.Fatalf("ai = %+v", ai)
	}
	if len(det.mimes) != 1 || det.mimes[0] != "image/jpeg" {
		t.Fatalf("detector calls = %v", det.mimes)
	}
	if v := Verdict(res, false); ai.Score >= scoring.AILikelyMin && !slices.Contains(v.FloorsApplied, scoring.FloorAILikely) {
		t.Fatalf("score %d without AI_LIKELY: %+v", ai.Score, v)
	}

	// A failing detector is not evidence: the card shows no detector line.
	p = newTestPipeline(true)
	p.Detector = &fakeDetector{err: errors.New("quota")}
	res = p.Run(context.Background(), imageRequest(t, 43, ""))
	if res.Pillars.AI.Detector != nil || !res.Pillars.AI.Available || res.Pillars.AI.Reliability != aiRelModel {
		t.Fatalf("ai = %+v", res.Pillars.AI)
	}
}

// WebP has no ELA (it is not JPEG-compressed), but everything else runs.
func TestWebPUploadSkipsELA(t *testing.T) {
	webp := append([]byte("RIFF\x16\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00"), 0, 0, 0, 0, 0x3F, 0, 0, 0x3F, 0, 0)
	var events []Event
	var mu sync.Mutex
	res := newTestPipeline(true).RunStream(context.Background(), Request{InputType: models.InputFile, Image: webp, ImageFormat: "webp"}, func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	if res.Pillars.ELA.Available || !res.Pillars.AI.Available || res.Degraded {
		t.Fatalf("ela=%v ai=%v degraded=%v", res.Pillars.ELA.Available, res.Pillars.AI.Available, res.Degraded)
	}
	for _, e := range events {
		if e.Pillar == PillarELA && e.Status != "skipped" {
			t.Fatalf("ELA event %+v", e)
		}
	}
}

// A link's preview image is judged too, but an AI illustration does not make
// a link a hoax: the AI floors are for uploads.
func TestLinkPreviewImageIsJudged(t *testing.T) {
	img := withC2PA(testJPEG(t, 44))
	p, srv := pipelineWithFetch(t, true, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cover.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(img)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>Banjir besar</title><meta property="og:image" content="/cover.jpg"></head></html>`))
		}
	})
	defer srv.Close()
	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: "https://berita.example/banjir"})
	ai := res.Pillars.AI
	if !ai.Available || ai.Provenance.Kind != models.ProvenanceAIGenerated {
		t.Fatalf("preview not judged: %+v", ai)
	}
	for _, f := range Verdict(res, false).FloorsApplied {
		if strings.HasPrefix(f, "AI_") {
			t.Fatalf("AI floor %s on a link", f)
		}
	}
}

func TestLinkWithoutPreviewHasNoAIPillar(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Tanpa gambar</title><meta property="og:image" content="/missing.jpg"></head></html>`))
	})
	defer srv.Close()
	var aiEvents []string
	var mu sync.Mutex
	res := p.RunStream(context.Background(), Request{InputType: models.InputURL, URL: "https://berita.example/a"}, func(e Event) {
		if e.Pillar == PillarAI {
			mu.Lock()
			aiEvents = append(aiEvents, e.Status)
			mu.Unlock()
		}
	})
	if res.Pillars.AI.Available || res.Degraded {
		t.Fatalf("ai=%+v degraded=%v", res.Pillars.AI, res.Degraded)
	}
	if !slices.Equal(aiEvents, []string{"running", "skipped"}) {
		t.Fatalf("ai events = %v", aiEvents)
	}
}
