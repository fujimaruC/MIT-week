// Package models mirrors api/openapi.yaml. Field names and JSON tags are a
// contract with the frontend Zod schemas; do not rename without updating
// api/openapi.yaml and frontend/src/lib/api in the same commit.
package models

// InputType is what the user submitted.
type InputType string

// AMBA checks two kinds of input: a link and an uploaded image.
const (
	InputURL  InputType = "URL"
	InputFile InputType = "FILE"
)

// Status is the verdict band computed by internal/scoring.
type Status string

const (
	StatusSafe         Status = "SAFE"
	StatusSuspicious   Status = "SUSPICIOUS"
	StatusTampered     Status = "TAMPERED"
	StatusInsufficient Status = "INSUFFICIENT"
)

// Dossier is the single result object returned by every audit route.
type Dossier struct {
	SchemaVersion   int       `json:"schema_version"`
	ID              string    `json:"id"`
	Timestamp       string    `json:"timestamp"`
	InputType       InputType `json:"input_type"`
	SourceReference string    `json:"source_reference"`
	Degraded        bool      `json:"degraded"`
	IsDemo          bool      `json:"is_demo"`
	Verdict         Verdict   `json:"verdict"`
	Pillars         Pillars   `json:"pillars"`
	Tactics         []Tactic  `json:"tactics"`
	ShareToken      string    `json:"share_token"`
	// SourceMetadata is the link's public description (PRD F1). It is null
	// for FILE input.
	SourceMetadata *SourceMetadata `json:"source_metadata"`
	// OCRText is the text the model read in an uploaded image ("" unless
	// FILE). Tactics with source "ocr" are highlighted in it.
	OCRText string `json:"ocr_text"`
	// Video is the frame-by-frame analysis of a social video link (TikTok,
	// YouTube, X, Instagram, Facebook). Absent for other input.
	Video *VideoAnalysis `json:"video,omitempty"`
}

// VideoStatus says whether a social video could be downloaded and analysed.
type VideoStatus string

const (
	VideoOK          VideoStatus = "OK"
	VideoUnavailable VideoStatus = "UNAVAILABLE"
)

// VideoAnalysis holds what only a video has; its AI score is in Pillars.AI.
// Reason is "" when Status is OK, else one of unsupported, private, too_long,
// too_big, timeout, download_failed, no_tools, model_failed.
type VideoAnalysis struct {
	Status          VideoStatus  `json:"status"`
	Reason          string       `json:"reason"`
	Platform        string       `json:"platform"`
	DurationSec     float64      `json:"duration_sec"`
	UploadDate      string       `json:"upload_date"`
	PlatformAILabel bool         `json:"platform_ai_label"`
	Frames          []VideoFrame `json:"frames"`
	AudioAnalyzed   bool         `json:"audio_analyzed"`
	Summary         string       `json:"summary"`
	Transcript      string       `json:"transcript"`
	Manipulation    VideoSignal  `json:"manipulation"`
	Context         VideoSignal  `json:"context"`
	// CreatorDisclosure is what the creator says about AI in the post.
	CreatorDisclosure CreatorDisclosure `json:"creator_disclosure"`
	// WebEvidence is reputable pages found by searching the video's own
	// words, at most 5, only those judged to be about this video.
	WebEvidence []WebEvidence `json:"web_evidence"`
	// W5H1 is what the video shows, as 5W1H; nil when the model gave none.
	W5H1 *W5H1 `json:"w5h1,omitempty"`
	// Research is the search on trusted newsrooms and fact-checkers for
	// what the video shows: the data behind the conclusion. Nil when no
	// search ran.
	Research *Research `json:"research,omitempty"`
}

// W5H1 is what the video shows: what, who, when, where, why and how, ""
// where the video does not show it (Sir, 10 Nov 2026).
type W5H1 struct {
	What  string `json:"what"`
	Who   string `json:"who"`
	When  string `json:"when"`
	Where string `json:"where"`
	Why   string `json:"why"`
	How   string `json:"how"`
}

// Research statuses.
const (
	ResearchFound       = "FOUND"
	ResearchNone        = "NONE"
	ResearchUnavailable = "UNAVAILABLE"
)

// Research is one search for the video's own content on trusted newsrooms
// and fact-checkers, and the pages it found (at most 5).
type Research struct {
	Query   string           `json:"query"`
	Status  string           `json:"status"`
	Sources []ResearchSource `json:"sources"`
}

// ResearchSource is one page the research found. Published is "" when the
// search did not say.
type ResearchSource struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Domain    string `json:"domain"`
	Snippet   string `json:"snippet"`
	Published string `json:"published"`
}

// Creator disclosure levels.
const (
	DisclosureNone   = "none"
	DisclosureWeak   = "weak"
	DisclosureStrong = "strong"
)

// CreatorDisclosure is the creator's own statement about AI (level none,
// weak or strong) and the text it came from.
type CreatorDisclosure struct {
	Level  string `json:"level"`
	Phrase string `json:"phrase"`
}

// What a web page says about a video (WebEvidence.Says).
const (
	WebSaysAI   = "AI"
	WebSaysReal = "REAL"
	WebSaysHoax = "HOAX"
)

// WebEvidence is one reputable page about the video.
type WebEvidence struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Publisher string `json:"publisher"`
	Says      string `json:"says"`
}

// VideoFrame is one analysed keyframe: a small JPEG data URL for the UI.
type VideoFrame struct {
	AtSec float64 `json:"at_sec"`
	Thumb string  `json:"thumb"`
}

// VideoSignal is a model-reported 0-100 signal with what it saw.
type VideoSignal struct {
	Score      int      `json:"score"`
	Indicators []string `json:"indicators"`
}

// MetadataStatus says whether a link's public description could be read.
type MetadataStatus string

const (
	MetadataOK          MetadataStatus = "OK"
	MetadataBlocked     MetadataStatus = "BLOCKED"
	MetadataUnavailable MetadataStatus = "UNAVAILABLE"
)

// SourceMetadata is what oEmbed/OpenGraph said about a link. BLOCKED means
// the platform refused automated reading and the UI asks for a screenshot.
type SourceMetadata struct {
	Status      MetadataStatus `json:"status"`
	Platform    string         `json:"platform"`
	Via         string         `json:"via"`
	Title       string         `json:"title"`
	Author      string         `json:"author"`
	Description string         `json:"description"`
	SiteName    string         `json:"site_name"`
}

// Verdict holds the computed risk assessment. Score is nil when the evidence
// was insufficient to produce one.
type Verdict struct {
	Score             *int     `json:"score"`
	Status            Status   `json:"status"`
	ShortTitle        string   `json:"short_title"`
	Explanation       string   `json:"explanation"`
	ExplanationSimple string   `json:"explanation_simple"`
	Basis             []string `json:"basis"`
	NotChecked        []string `json:"not_checked"`
	FloorsApplied     []string `json:"floors_applied"`
}

// Pillars holds the four independent evidence sources.
type Pillars struct {
	ELA  ELAPillar  `json:"ela"`
	AI   AIPillar   `json:"ai"`
	Fact FactPillar `json:"fact"`
	Link LinkPillar `json:"link"`
}

// ELAPillar is server-side image forensics.
type ELAPillar struct {
	Available       bool      `json:"available"`
	Score           int       `json:"score"`
	Reliability     float64   `json:"reliability"`
	ReliabilityNote *string   `json:"reliability_note"`
	Anomalies       []Anomaly `json:"anomalies"`
}

// Anomaly is a normalized 0-1000 box in [ymin, xmin, ymax, xmax] order:
// text the model says looks pasted. Evidence (0-100) is how differently that
// text reacts to re-compression compared with the image's other text
// (internal/forensics TextEvidence); nil when it could not be measured.
type Anomaly struct {
	BoundingBox   [4]int `json:"bounding_box"`
	ExtractedText string `json:"extracted_text"`
	Reason        string `json:"reason"`
	Evidence      *int   `json:"evidence"`
}

// AIPillar ("Jejak AI") answers whether the image was made with AI: the
// uploaded image, or a link's preview image. Score is the likelihood (0-100)
// computed in Go from the file's provenance, an optional dedicated detector
// and the vision model's visible clues; Reliability is how much the strongest
// of those deserves to weigh.
type AIPillar struct {
	Available   bool        `json:"available"`
	Score       int         `json:"score"`
	Reliability float64     `json:"reliability"`
	Indicators  []string    `json:"indicators"`
	Caveat      string      `json:"caveat"`
	Provenance  Provenance  `json:"provenance"`
	Detector    *AIDetector `json:"detector"`
}

// ProvenanceKind is what an image declares about its origin.
type ProvenanceKind string

const (
	ProvenanceAIGenerated ProvenanceKind = "AI_GENERATED"
	ProvenanceAIEdited    ProvenanceKind = "AI_EDITED"
	ProvenanceCamera      ProvenanceKind = "CAMERA"
	ProvenanceNone        ProvenanceKind = "NONE"
)

// Provenance is what the image declares about where it came from: Content
// Credentials (c2pa), IPTC/XMP metadata (xmp), generator settings in PNG text
// (png_text), EXIF (exif), or an AI label visible on the image (watermark).
// Source "creator" is the creator's own AI disclosure and "web" is news
// coverage of the video (video links only). Generator and Camera are "" when
// not named.
type Provenance struct {
	Kind      ProvenanceKind `json:"kind"`
	Source    string         `json:"source"`
	Generator string         `json:"generator"`
	Camera    string         `json:"camera"`
	C2PA      bool           `json:"c2pa"`
}

// AIDetector is a dedicated AI-image detector's answer (0-100), present only
// when one is configured and answered.
type AIDetector struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

// FactStatus is the outcome of checking the claim against allowlisted sources.
type FactStatus string

const (
	FactContradicts FactStatus = "CONTRADICTS"
	FactSupports    FactStatus = "SUPPORTS"
	FactUnverified  FactStatus = "UNVERIFIED"
)

// FactPillar is claim verification against trusted publishers.
type FactPillar struct {
	Available bool       `json:"available"`
	Claim     string     `json:"claim"`
	Status    FactStatus `json:"status"`
	Sources   []Source   `json:"sources"`
}

// Source is a verified publisher reference.
type Source struct {
	Title     string     `json:"title"`
	Publisher string     `json:"publisher"`
	URL       string     `json:"url"`
	Match     FactStatus `json:"match"`
}

// LinkPillar is URL heuristic and reputation analysis.
type LinkPillar struct {
	Available     bool         `json:"available"`
	Score         int          `json:"score"`
	FinalURL      string       `json:"final_url"`
	RedirectChain []string     `json:"redirect_chain"`
	Reasons       []LinkReason `json:"reasons"`
}

// LinkReason is one human-readable finding from link analysis.
type LinkReason struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

// Tactic is an informational persuasion pattern. It never affects the score.
type Tactic struct {
	Code   string `json:"code"`
	Phrase string `json:"phrase"`
	Source string `json:"source"`
}

// ErrorEnvelope is the only error shape the API ever returns.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody carries a stable code, an Indonesian message and a retry hint.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Error codes shared with the frontend.
const (
	CodeInvalidURL       = "INVALID_URL"
	CodeBlockedTarget    = "BLOCKED_TARGET"
	CodeUnsupportedMedia = "UNSUPPORTED_MEDIA"
	CodePayloadTooLarge  = "PAYLOAD_TOO_LARGE"
	CodeRateLimited      = "RATE_LIMITED"
	CodeUpstreamTimeout  = "UPSTREAM_TIMEOUT"
	CodeInternal         = "INTERNAL"
)
