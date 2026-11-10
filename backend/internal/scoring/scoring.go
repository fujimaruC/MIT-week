// Package scoring implements AMBA's deterministic risk score.
//
// It is pure: no I/O, no clocks, no randomness. The LLM never produces a
// score; this package combines the four pillar signals and applies the
// floors, caps and status bands from TECH_SPEC §3.2–§3.3.
package scoring

import (
	"math"
	"slices"

	"github.com/amba/amba/backend/internal/models"
)

// Base pillar weights (TECH_SPEC §3.1).
const (
	WeightELA  = 0.30
	WeightAI   = 0.15
	WeightFact = 0.35
	WeightLink = 0.20

	// MinWeight is the smallest total effective weight that yields a weighted
	// score. Below it the verdict is SAFE (score held at SafeMax) unless a floor fires.
	MinWeight = 0.30

	// LinkStrongMin is the link score from which link evidence alone may
	// produce a verdict when nothing else is available (product decision,
	// November 2026: a clear phishing link must not read "belum cukup data").
	// 45 is a brand-impersonating host on its own: in a public phishing feed
	// more than half of the Indonesian-themed links scored exactly that, while
	// no popular legitimate site reached it.
	LinkStrongMin = 45
	// LinkOnlyCap keeps a link-only verdict at or below the LINK_MALICIOUS
	// floor, so the score never rises as the evidence gets thinner.
	LinkOnlyCap = 85
	// ELAEvidenceMin is the ELA score from which image analysis counts as
	// evidence. ELA only ever counts as evidence of editing: on real images
	// (November 2026) untouched photos scored 0 and edited ones 0-14, so a
	// low score says nothing about authenticity. Below this it neither lowers
	// the score nor helps a verdict reach SAFE; it only counts as coverage.
	ELAEvidenceMin = 50
	// AILikelyMin is the AI pillar score from which an uploaded image is
	// SUSPICIOUS on visible signs alone (AI_LIKELY). The prompt reserves
	// 71-100 for clear signs (malformed hands, gibberish lettering, impossible
	// objects, AI labels); a configured detector shares the score.
	AILikelyMin = 80
	// The video thresholds were calibrated on 33 labelled clips (11 real
	// news, 10 AI, 12 hoax) scored in three runs (10 Nov 2026; numbers in
	// BE-NOTES.md). The model ranks AI clips well (AUC 0.90) but scores the
	// realistic ones 65-75, so a video needs a lower AI line than an image:
	// at 80 only 23% of AI clips were flagged, at 55 53% were, with 1 of 33
	// real-news readings (3%) flagged. Real news never scored above 60 for
	// manipulation, but twice scored 75-80 for context mismatch, so that
	// signal needs a higher line. None of these is confirmed by an outside
	// source, so they stop at SUSPICIOUS.
	VideoAILikelyMin     = 55
	VideoManipulationMin = 70
	VideoContextMin      = 85

	// SafeMax is the highest score of a SAFE verdict.
	SafeMax = 20

	// weightEpsilon absorbs binary floating-point error in the weight sums.
	// 0.35 + 0.15 is 0.49999999999999994, which must still count as 0.50.
	weightEpsilon = 1e-9
)

// Floor codes as they appear in verdict.floors_applied (api/openapi.yaml).
const (
	FloorFactContradicted = "FACT_CONTRADICTED"
	FloorLinkMalicious    = "LINK_MALICIOUS"
	FloorELACorroborated  = "ELA_CORROBORATED"
	FloorLinkStrong       = "LINK_STRONG"
	FloorELASoloCap       = "ELA_SOLO_CAP"
	FloorInjectionGuard   = "INJECTION_GUARD"
	// The AI floors apply to an uploaded image or a downloaded social video
	// only: a link's preview image may be an honest illustration.
	FloorAIProvenance = "AI_PROVENANCE"
	FloorAIEdited     = "AI_EDITED"
	FloorAILikely     = "AI_LIKELY"
	// Video floors, from the model's frame-by-frame findings.
	FloorVideoManipulated     = "VIDEO_MANIPULATED"
	FloorVideoContextMismatch = "VIDEO_CONTEXT_MISMATCH"
)

// AI origins an uploaded image can declare (Input.AIDeclared).
const (
	DeclaredAIGenerated = "AI_GENERATED"
	DeclaredAIEdited    = "AI_EDITED"
)

// Signal is one pillar's contribution. Unavailable pillars contribute neither
// weight nor score.
type Signal struct {
	Available   bool
	Score       int
	Reliability float64
}

// Box is a normalized [ymin, xmin, ymax, xmax] rectangle on a 0–1000 scale,
// matching the Dossier anomaly contract.
type Box [4]int

// Input is everything Compute needs. The four Signals come from the pillars;
// the remaining fields are the evidence the floors inspect.
type Input struct {
	ELA  Signal
	AI   Signal
	Fact Signal
	Link Signal

	// FactContradicted is true when at least one allowlisted source returned
	// match=CONTRADICTS. Allowlist verification happens in internal/grounding.
	FactContradicted bool
	// LinkMalicious is true on a Safe Browsing hit. A link score >= 90 is
	// detected from Link directly.
	LinkMalicious bool
	// ELAHotspots are the high-difference boxes reported by internal/forensics.
	ELAHotspots []Box
	// AIAnomalies are the LLM's text-anomaly boxes.
	AIAnomalies []Box
	// InjectionSuspected is true when the LLM output was flagged by the
	// injection detector.
	InjectionSuspected bool

	// UploadedImage is true when the user uploaded the image the AI pillar
	// judged. Only then do the AI floors apply.
	UploadedImage bool
	// AIDeclared is what the image declares about its origin, from its
	// metadata or an AI label visible on it: DeclaredAIGenerated,
	// DeclaredAIEdited or "".
	AIDeclared string

	// Video is true when the AI pillar judged the keyframes of a downloaded
	// social video. The AI floors then apply as for an uploaded image (its
	// AIDeclared comes from the platform's AI label or a visible AI mark).
	Video bool
	// PreviewImage is true when a video link's footage could not be
	// downloaded and the AI pillar judged its preview picture instead, with a
	// score at the photo bar (AILikelyMin). The AI floors then apply as for an
	// uploaded image: a reason to check, never more than SUSPICIOUS on its own.
	PreviewImage bool
	// VideoManipulation and VideoContext are the model's 0-100 signals for a
	// video edited to mislead and for a caption the footage does not support.
	VideoManipulation int
	VideoContext      int
}

// Compute returns the verdict fields this package owns. ShortTitle and
// Explanation are presentation copy filled by the caller.
func Compute(in Input) models.Verdict {
	v := models.Verdict{
		Basis: []string{},
		// NotChecked lists the pillars that had no result; the pipeline narrows it
		// to the checks that apply to the input and failed.
		NotChecked:    []string{},
		FloorsApplied: []string{},
	}

	// One-sided ELA: an inconclusive image analysis is reported as checked
	// but carries no weight in the score.
	wELA := effWeight(in.ELA, WeightELA)
	if in.ELA.Score < ELAEvidenceMin {
		wELA = 0
	}
	wAI := effWeight(in.AI, WeightAI)
	wFact := effWeight(in.Fact, WeightFact)
	wLink := effWeight(in.Link, WeightLink)
	totalW := wELA + wAI + wFact + wLink

	if in.ELA.Available {
		v.Basis = append(v.Basis, "ela")
	} else {
		v.NotChecked = append(v.NotChecked, "ela")
	}
	if in.AI.Available {
		v.Basis = append(v.Basis, "ai")
	} else {
		v.NotChecked = append(v.NotChecked, "ai")
	}
	if in.Fact.Available {
		v.Basis = append(v.Basis, "fact")
	} else {
		v.NotChecked = append(v.NotChecked, "fact")
	}
	if in.Link.Available {
		v.Basis = append(v.Basis, "link")
	} else {
		v.NotChecked = append(v.NotChecked, "link")
	}

	var score *int
	if totalW+weightEpsilon >= MinWeight {
		raw := (wELA*float64(in.ELA.Score) +
			wAI*float64(in.AI.Score) +
			wFact*float64(in.Fact.Score) +
			wLink*float64(in.Link.Score)) / totalW
		s := int(math.Floor(raw + 0.5))
		score = &s
	}

	// Floors run in TECH_SPEC §3.3 order. They can rescue an otherwise
	// INSUFFICIENT input, but never to SAFE.
	if in.FactContradicted {
		score = atLeast(score, 85)
		v.FloorsApplied = append(v.FloorsApplied, FloorFactContradicted)
	}
	if in.LinkMalicious || (in.Link.Available && in.Link.Score >= 90) {
		score = atLeast(score, 85)
		v.FloorsApplied = append(v.FloorsApplied, FloorLinkMalicious)
	}
	if in.ELA.Available && in.ELA.Score >= 70 && boxesOverlap(in.ELAHotspots, in.AIAnomalies) {
		score = atLeast(score, 70)
		v.FloorsApplied = append(v.FloorsApplied, FloorELACorroborated)
	}
	// LINK_STRONG: when the evidence is too thin for a weighted score but the
	// link alone is clearly risky, the link score (capped) becomes the score.
	// It only rescues an INSUFFICIENT result and can never produce SAFE.
	if score == nil && in.Link.Available && in.Link.Score >= LinkStrongMin {
		s := min(in.Link.Score, LinkOnlyCap)
		score = &s
		v.FloorsApplied = append(v.FloorsApplied, FloorLinkStrong)
	}
	// An uploaded image that declares AI generation is not a photo of a real
	// event, whatever else it shows; one edited with AI is altered; one whose
	// visible signs are clear deserves a check before it is shared. These
	// come after LINK_STRONG so a risky link in the image keeps its score.
	if (in.UploadedImage || in.Video || in.PreviewImage) && in.AI.Available {
		switch {
		case in.AIDeclared == DeclaredAIGenerated:
			score = atLeast(score, 85)
			v.FloorsApplied = append(v.FloorsApplied, FloorAIProvenance)
		case in.AIDeclared == DeclaredAIEdited:
			score = atLeast(score, 70)
			v.FloorsApplied = append(v.FloorsApplied, FloorAIEdited)
		case in.AI.Score >= aiLikelyMin(in):
			score = atLeast(score, 45)
			v.FloorsApplied = append(v.FloorsApplied, FloorAILikely)
		}
	}
	// A video edited to mislead, or a caption its footage does not support:
	// the model's own signals, so they stop at SUSPICIOUS.
	if in.Video {
		if in.VideoManipulation >= VideoManipulationMin {
			score = atLeast(score, 45)
			v.FloorsApplied = append(v.FloorsApplied, FloorVideoManipulated)
		}
		if in.VideoContext >= VideoContextMin {
			score = atLeast(score, 45)
			v.FloorsApplied = append(v.FloorsApplied, FloorVideoContextMismatch)
		}
	}
	// The solo cap is recorded only when it actually lowers the score: it is a
	// numerical ceiling, not evidence, so listing it for a score already below
	// 60 would make the honest-limits copy claim a cap that never mattered.
	if in.ELA.Available && len(v.FloorsApplied) == 0 && score != nil &&
		!otherPillarAtLeast50(in) && *score > 60 {
		s := 60
		score = &s
		v.FloorsApplied = append(v.FloorsApplied, FloorELASoloCap)
	}
	if in.InjectionSuspected {
		score = atLeast(score, 40)
		v.FloorsApplied = append(v.FloorsApplied, FloorInjectionGuard)
		v.Basis = append(v.Basis, "llm_untrusted")
	}

	// A finished audit always ends in a verdict (Sir, 10 Nov: no "belum cukup
	// data"). Evidence too thin for a weighted score and no floor: SAFE, with the
	// weighted score of what was checked held at or below the SAFE line. The
	// floors above ran on a nil score, so they decide exactly as before.
	if score == nil {
		s := 0
		if totalW > 0 {
			raw := (wELA*float64(in.ELA.Score) + wAI*float64(in.AI.Score) +
				wFact*float64(in.Fact.Score) + wLink*float64(in.Link.Score)) / totalW
			s = min(int(math.Floor(raw+0.5)), SafeMax)
		}
		score = &s
	}

	if score != nil {
		s := *score
		if s < 0 {
			s = 0
		}
		if s > 100 {
			s = 100
		}
		score = &s
	}

	switch {
	case *score <= SafeMax:
		v.Status = models.StatusSafe
	case *score <= 60:
		v.Status = models.StatusSuspicious
	default:
		v.Status = models.StatusTampered
	}

	v.Score = score
	v.ExplanationSimple = explanationSimple(v.Status, v.FloorsApplied, in.Video)
	return v
}

// / aiLikelyMin is the AI score from which visible signs alone are enough:
// the lower video line for a downloaded clip, AILikelyMin for an image.
func aiLikelyMin(in Input) int {
	if in.Video {
		return VideoAILikelyMin
	}
	return AILikelyMin
}

// ExplanationSimple returns the deterministic lead sentence from TECH_SPEC
// §8.2. Floor-specific sentences win over the generic status sentence.
func ExplanationSimple(status models.Status, floors []string) string {
	return explanationSimple(status, floors, false)
}

// explanationSimple names a video, not an image, when the AI pillar judged
// a downloaded social video.
func explanationSimple(status models.Status, floors []string, video bool) string {
	switch {
	case slices.Contains(floors, FloorFactContradicted):
		return "Kabar ini dibantah sumber terpercaya. Jangan disebarkan."
	case slices.Contains(floors, FloorLinkMalicious):
		return "Tautan ini mirip penipuan. Jangan diklik."
	case slices.Contains(floors, FloorLinkStrong):
		return "Tautan ini punya tanda penipuan. Sebaiknya jangan diklik."
	case slices.Contains(floors, FloorAIProvenance):
		if video {
			return "Ada tanda jelas video ini dibuat dengan AI. Jangan anggap sebagai rekaman asli."
		}
		return "Ada tanda jelas gambar ini dibuat dengan AI. Jangan anggap sebagai foto asli."
	case slices.Contains(floors, FloorELACorroborated):
		return "Gambar ini terindikasi diedit. Jangan langsung percaya."
	case slices.Contains(floors, FloorAIEdited):
		return "Ada tanda jelas gambar ini diedit dengan AI. Jangan langsung percaya."
	case slices.Contains(floors, FloorAILikely):
		if video {
			return "Video ini tampak buatan AI. Cek dulu sebelum menyebarkan."
		}
		return "Gambar ini tampak buatan AI. Cek dulu sebelum menyebarkan."
	case slices.Contains(floors, FloorVideoManipulated):
		return "Video ini tampak disunting untuk menyesatkan. Cek dulu sebelum menyebarkan."
	case slices.Contains(floors, FloorVideoContextMismatch):
		return "Keterangan video ini tidak cocok dengan isi rekamannya. Cek dulu sebelum menyebarkan."
	}

	switch status {
	case models.StatusSafe:
		return "Tidak ditemukan tanda bahaya, tapi tetap bijak sebelum menyebarkan."
	case models.StatusSuspicious:
		return "Ada tanda yang perlu diwaspadai. Cek dulu sebelum menyebarkan."
	case models.StatusTampered:
		// §8.2 defines TAMPERED sentences for the floors above. A weighted
		// score can reach TAMPERED without one (e.g. ELA 69 + AI 100 +
		// UNVERIFIED + link 89), so keep a calm, non-certain fallback.
		return "Beberapa tanda bahaya ditemukan. Cek dulu sebelum menyebarkan."
	default: // INSUFFICIENT: only in old payloads
		return "Tidak ditemukan tanda bahaya, tapi tetap bijak sebelum menyebarkan."
	}
}

// overlaps reports whether an anomaly box overlaps an ELA hotspot: IoU >= 0.1
// or either box's centre lies inside the other.
func overlaps(a, b Box) bool {
	ay0, ax0, ay1, ax1 := ordered(a)
	by0, bx0, by1, bx1 := ordered(b)
	// A degenerate box (zero width or height) is not evidence of anything.
	if ay1 <= ay0 || ax1 <= ax0 || by1 <= by0 || bx1 <= bx0 {
		return false
	}

	iw := min(ax1, bx1) - max(ax0, bx0)
	ih := min(ay1, by1) - max(ay0, by0)
	if iw > 0 && ih > 0 {
		inter := float64(iw * ih)
		union := rectArea(ay0, ax0, ay1, ax1) + rectArea(by0, bx0, by1, bx1) - inter
		if union > 0 && inter/union >= 0.1 {
			return true
		}
	}

	by, bx := centre(b)
	ay, ax := centre(a)
	return containsPoint(ay0, ax0, ay1, ax1, by, bx) ||
		containsPoint(by0, bx0, by1, bx1, ay, ax)
}

// effWeight returns base*reliability for an available pillar. The contract
// says reliability is 0–1; clamping keeps a bad caller from inverting or
// inflating the weights.
func effWeight(s Signal, base float64) float64 {
	if !s.Available {
		return 0
	}
	r := s.Reliability
	if !(r >= 0) { // also rejects NaN
		r = 0
	}
	if r > 1 {
		r = 1
	}
	return base * r
}

func atLeast(score *int, floor int) *int {
	if score == nil || *score < floor {
		s := floor
		return &s
	}
	return score
}

func otherPillarAtLeast50(in Input) bool {
	return (in.AI.Available && in.AI.Score >= 50) ||
		(in.Fact.Available && in.Fact.Score >= 50) ||
		(in.Link.Available && in.Link.Score >= 50)
}

func boxesOverlap(hotspots, anomalies []Box) bool {
	for _, h := range hotspots {
		for _, a := range anomalies {
			if overlaps(h, a) {
				return true
			}
		}
	}
	return false
}

func ordered(b Box) (y0, x0, y1, x1 int) {
	y0, x0, y1, x1 = b[0], b[1], b[2], b[3]
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	return
}

func rectArea(y0, x0, y1, x1 int) float64 {
	return float64(y1-y0) * float64(x1-x0)
}

func centre(b Box) (y, x float64) {
	y0, x0, y1, x1 := ordered(b)
	return float64(y0+y1) / 2, float64(x0+x1) / 2
}

func containsPoint(y0, x0, y1, x1 int, py, px float64) bool {
	return py >= float64(y0) && py <= float64(y1) &&
		px >= float64(x0) && px <= float64(x1)
}
