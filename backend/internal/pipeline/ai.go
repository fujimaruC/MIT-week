package pipeline

import (
	"bytes"
	"context"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/metadata"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/provenance"
	"github.com/amba/amba/backend/internal/security"
)

// The AI pillar ("Jejak AI") answers whether an image was made with AI. Its
// score is computed here from three kinds of evidence, strongest first:
//
//  1. what the image declares: Content Credentials, IPTC/XMP, generator
//     settings or EXIF (internal/provenance), or an AI label that both the
//     model and the text read from the image show;
//  2. a dedicated detector, when one is configured (internal/aidetect);
//  3. the vision model's visible clues (ai_score and ai_indicators).
//
// Missing metadata proves nothing (WhatsApp and social networks strip it),
// and camera metadata is a hint, not proof: anyone can write it.
const (
	aiRelDeclared = 1.0
	aiRelLabel    = 0.9
	aiRelDetector = 0.8
	aiRelModel    = 0.5

	// aiDetectorShare is the detector's share of the score when the model
	// answered too: a purpose-built detector beats a general vision model.
	aiDetectorShare = 0.7
	// cameraCap keeps a visual judgement at "uncertain" when the file names
	// a camera and carries no AI marker.
	cameraCap = 50
	// maxAIIndicators bounds the clues shown on the card.
	maxAIIndicators = 5

	// weakDisclosureBoost is added when the creator only mentions AI.
	weakDisclosureBoost = 8

	aiCaveat = "Indikatif, bukan bukti pasti"
)

// AIDetector is a dedicated AI-image detector (internal/aidetect).
type AIDetector interface {
	Enabled() bool
	Name() string
	Score(ctx context.Context, image []byte, mime string) (int, error)
}

// detection is one detector answer; ok=false when none was given.
type detection struct {
	name  string
	score int
	ok    bool
}

// aiEvidence is everything the AI pillar weighs for one image.
type aiEvidence struct {
	prov     provenance.Result
	modelOK  bool
	findings llm.Findings
	// ocr is the text read in the image, to confirm a visible AI label.
	ocr string
	det detection
	// weakDisclosure: the creator only mentions AI (#ai). A small boost.
	weakDisclosure bool
}

// aiLabel matches the explicit "made with AI" labels that generators and
// platforms put on images, in the languages AMBA's users see.
var aiLabel = regexp.MustCompile(`(?i)(\bai[- ]?generated\b|\bgenerated (?:by|with) ai\b|\b(?:made|created|imagined) with ai\b|\bdibuat (?:dengan|oleh|menggunakan|pakai) ai\b|\bdihasilkan (?:oleh |dengan )?ai\b|AI\s*生成)`)

// visibleAILabel is true when the model reports an AI label on the image and
// the text read from the image carries one too: two readings agree.
func visibleAILabel(watermark, ocr string) bool {
	return aiLabel.MatchString(watermark) && aiLabel.MatchString(ocr)
}

// buildAIPillar computes the AI pillar. ok=false means nothing could judge
// the image (the model failed, no detector answered, nothing declared).
func buildAIPillar(ev aiEvidence) (models.AIPillar, bool) {
	p := models.AIPillar{
		Indicators: []string{},
		Caveat:     aiCaveat,
		Provenance: provenanceOf(ev.prov),
	}
	label := ev.modelOK && visibleAILabel(ev.findings.AIWatermark, ev.ocr)
	if !ev.modelOK && !ev.det.ok && !ev.prov.AI() {
		return models.AIPillar{Indicators: []string{}, Caveat: aiCaveat, Provenance: p.Provenance}, false
	}

	score, rel := 0, 0.0
	if ev.modelOK {
		score, rel = ev.findings.AIScore, aiRelModel
		p.Indicators = append(p.Indicators, ev.findings.AIIndicators...)
		if w := ev.findings.AIWatermark; w != "" && !label && !mentions(p.Indicators, w) {
			p.Indicators = append(p.Indicators, "Tampak tanda air AI: "+w)
		}
		if len(p.Indicators) > maxAIIndicators {
			p.Indicators = p.Indicators[:maxAIIndicators]
		}
	}
	if ev.det.ok {
		p.Detector = &models.AIDetector{Name: ev.det.name, Score: ev.det.score}
		if ev.modelOK {
			score = int(math.Round(aiDetectorShare*float64(ev.det.score) + (1-aiDetectorShare)*float64(score)))
		} else {
			score = ev.det.score
		}
		rel = aiRelDetector
	}
	switch {
	case ev.prov.Kind == provenance.AIGenerated:
		score, rel = max(score, 95), aiRelDeclared
	case ev.prov.Kind == provenance.AIEdited:
		score, rel = max(score, 80), aiRelDeclared
	case label:
		score, rel = max(score, 90), aiRelLabel
		p.Provenance.Kind = models.ProvenanceAIGenerated
		p.Provenance.Source = "watermark"
		p.Provenance.Generator = provenance.GeneratorIn(ev.findings.AIWatermark)
	case ev.prov.Kind == provenance.Camera:
		score = min(score, cameraCap)
	case ev.weakDisclosure && ev.modelOK:
		score += weakDisclosureBoost
	}
	p.Available, p.Score, p.Reliability = true, min(max(score, 0), 100), rel
	return p, true
}

func provenanceOf(r provenance.Result) models.Provenance {
	kind := models.ProvenanceNone
	switch r.Kind {
	case provenance.AIGenerated:
		kind = models.ProvenanceAIGenerated
	case provenance.AIEdited:
		kind = models.ProvenanceAIEdited
	case provenance.Camera:
		kind = models.ProvenanceCamera
	}
	return models.Provenance{Kind: kind, Source: r.Source, Generator: r.Generator, Camera: r.Camera, C2PA: r.C2PA}
}

func mentions(list []string, s string) bool {
	s = strings.ToLower(s)
	return slices.ContainsFunc(list, func(item string) bool { return strings.Contains(strings.ToLower(item), s) })
}

// startDetector runs the configured detector beside the model. It returns
// nil when no detector is configured; otherwise the channel receives exactly
// one answer.
func (p *Pipeline) startDetector(ctx context.Context, image []byte, format string) <-chan detection {
	if p.Detector == nil || !p.Detector.Enabled() || len(image) == 0 {
		return nil
	}
	ch := make(chan detection, 1)
	go func() {
		dctx, cancel := withBudget(ctx, p.DetectDeadline)
		defer cancel()
		score, err := p.Detector.Score(dctx, image, imageMIME(format))
		ch <- detection{name: p.Detector.Name(), score: score, ok: err == nil}
	}()
	return ch
}

func awaitDetection(ctx context.Context, ch <-chan detection) detection {
	if ch == nil {
		return detection{}
	}
	select {
	case d := <-ch:
		return d
	case <-ctx.Done():
		return detection{}
	}
}

// previewMaxBytes bounds a link's preview image download.
const previewMaxBytes = 5 << 20

// fetchPreview downloads a link's preview image through the SSRF-safe client
// and checks it exactly like an upload: same formats, same pixel limits.
func (p *Pipeline) fetchPreview(ctx context.Context, imageURL string) ([]byte, string, bool) {
	if _, err := security.ValidateFetchURL(imageURL); err != nil {
		return nil, "", false
	}
	budget := p.PreviewDeadline
	if budget <= 0 {
		budget = 1500 * time.Millisecond
	}
	pctx, cancel := withBudget(ctx, budget)
	defer cancel()
	body, err := metadata.New(p.NewFetchClient(budget)).PreviewImage(pctx, imageURL, previewMaxBytes)
	if err != nil {
		return nil, "", false
	}
	data, format, _, err := security.ValidateUpload(bytes.NewReader(body), previewMaxBytes)
	if err != nil {
		return nil, "", false
	}
	return data, format, true
}

func imageMIME(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
