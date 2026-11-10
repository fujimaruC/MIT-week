package pipeline

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/amba/amba/backend/internal/forensics"
	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/video"
)

// VideoSource downloads a social video for analysis (internal/video.Fetcher).
type VideoSource interface {
	Enabled() bool
	Fetch(ctx context.Context, rawURL string) (*video.Clip, error)
}

const (
	// videoThumbSide is the longest side of the frame thumbnails in the Dossier.
	videoThumbSide = 240
	// videoAfterModel is what a video audit keeps after the model answers:
	// the claim search and classification, and the verdict.
	videoAfterModel = 6 * time.Second
	// maxVideoDownloads caps concurrent downloads: each runs yt-dlp and ffmpeg.
	maxVideoDownloads = 3
)

// fetchVideo downloads a supported social video link. Any other link, or a
// pipeline without a video source, returns nil, nil. A failed download
// returns the UNAVAILABLE object with its reason; transient says whether a
// retry could succeed, so the result is not cached. A failed download that
// still learned the post's metadata returns a metadata-only clip (no Frames)
// together with the UNAVAILABLE object.
func (p *Pipeline) fetchVideo(ctx context.Context, rawURL string) (clip *video.Clip, va *models.VideoAnalysis, transient bool) {
	if p.Video == nil || !p.Video.Enabled() {
		return nil, nil, false
	}
	platform, ok := video.Platform(rawURL)
	if !ok {
		return nil, nil, false
	}
	va = &models.VideoAnalysis{
		Status: models.VideoUnavailable, Platform: platform, Frames: []models.VideoFrame{},
		Manipulation:      models.VideoSignal{Indicators: []string{}},
		Context:           models.VideoSignal{Indicators: []string{}},
		CreatorDisclosure: models.CreatorDisclosure{Level: models.DisclosureNone},
		WebEvidence:       []models.WebEvidence{},
	}
	select {
	case p.videoSlots <- struct{}{}:
		defer func() { <-p.videoSlots }()
	case <-ctx.Done():
		va.Reason = "timeout"
		return nil, va, true
	}
	fctx, cancel := withBudget(ctx, p.VideoFetchDeadline)
	defer cancel()
	c, err := p.Video.Fetch(fctx, rawURL)
	// TikTok sometimes stalls one socket read (yt-dlp gives up after 10 s);
	// the same video went through on the next try. Retry once while at
	// least 10 s of the download budget is left.
	if errors.Is(err, video.ErrTimeout) && fctx.Err() == nil {
		if dl, ok := fctx.Deadline(); !ok || time.Until(dl) >= 10*time.Second {
			retry, rerr := p.Video.Fetch(fctx, rawURL)
			if retry != nil {
				c = retry
			}
			err = rerr
		}
	}
	if err != nil || c == nil || len(c.Frames) == 0 {
		if err == nil {
			err = video.ErrDownload
		}
		va.Reason, transient = videoReason(err)
		if c != nil {
			c.Frames = nil // metadata only: the title and caption are still evidence
		}
		return c, va, transient
	}
	return c, va, false
}

// videoReason maps a fetch error to the Dossier's reason code.
func videoReason(err error) (reason string, transient bool) {
	switch {
	case errors.Is(err, video.ErrUnsupported):
		return "unsupported", false
	case errors.Is(err, video.ErrPrivate):
		return "private", false
	case errors.Is(err, video.ErrTooLong):
		return "too_long", false
	case errors.Is(err, video.ErrTooBig):
		return "too_big", false
	case errors.Is(err, video.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout", true
	case errors.Is(err, video.ErrNoTools):
		return "no_tools", true
	default:
		return "download_failed", true
	}
}

// videoInput is what the model sees of a clip.
func videoInput(c *video.Clip) llm.VideoInput {
	in := llm.VideoInput{
		Platform: c.Platform, Title: c.Title, Description: c.Description, Uploader: c.Uploader,
		UploadDate: c.UploadDate, DurationSec: c.DurationSec, AILabel: c.AILabel,
		Subtitles: c.Subtitles, Audio: c.Audio,
	}
	for _, f := range c.Frames {
		in.Frames = append(in.Frames, f.JPEG)
		in.FrameTimes = append(in.FrameTimes, f.AtSec)
	}
	return in
}

// videoFindings maps a video's findings onto the image findings the rest of
// the pipeline already handles (AI pillar, tactics, claim check).
func videoFindings(vf llm.VideoFindings) llm.Findings {
	return llm.Findings{
		Claim: vf.Claim, SearchQuery: vf.SearchQuery,
		AIScore: vf.AIScore, AIIndicators: vf.AIIndicators, AIWatermark: vf.AIWatermark,
		TextAnomalies: []llm.Anomaly{}, OCRText: vf.OCRText,
		Tactics: vf.Tactics, InjectionSuspected: vf.InjectionSuspected,
	}
}

// videoTacticSource is the text a video's tactic phrases must be quoted from.
func videoTacticSource(c *video.Clip, vf llm.VideoFindings, caption string) string {
	return strings.Join([]string{c.Title, c.Description, c.Subtitles, vf.Transcript, vf.OCRText, caption}, "\n")
}

// fillVideo completes the Dossier's video object from a clip and, when the
// model answered, its findings.
func fillVideo(va *models.VideoAnalysis, c *video.Clip, vf *llm.VideoFindings) {
	va.DurationSec = math.Round(c.DurationSec*10) / 10
	va.UploadDate = isoDate(c.UploadDate)
	va.PlatformAILabel = c.AILabel
	va.Frames = va.Frames[:0]
	for _, f := range c.Frames {
		thumb, mime := forensics.ModelImage(f.JPEG, "jpeg", videoThumbSide)
		if mime != "image/jpeg" {
			continue
		}
		va.Frames = append(va.Frames, models.VideoFrame{
			AtSec: math.Round(f.AtSec*10) / 10,
			Thumb: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(thumb),
		})
	}
	if vf == nil {
		va.Status, va.Reason = models.VideoUnavailable, "model_failed"
		return
	}
	va.Status, va.Reason = models.VideoOK, ""
	va.AudioAnalyzed = vf.AudioHeard
	va.Summary, va.Transcript = vf.Summary, vf.Transcript
	va.Manipulation = models.VideoSignal{Score: vf.ManipulationScore, Indicators: nonNilStrings(vf.ManipulationIndicators)}
	va.Context = models.VideoSignal{Score: vf.ContextMismatchScore, Indicators: nonNilStrings(vf.ContextIndicators)}
	va.W5H1 = nil
	if !vf.W5H1.Empty() {
		w := models.W5H1(vf.W5H1)
		va.W5H1 = &w
	}
}

// videoModelBudget is the model's share of a video audit: what is left of
// the request after the download, minus what runs after the model.
func (p *Pipeline) videoModelBudget(ctx context.Context) time.Duration {
	if dl, ok := ctx.Deadline(); ok {
		if d := time.Until(dl) - videoAfterModel; d > p.LLMDeadline {
			return d
		}
	}
	return p.LLMDeadline
}

func isoDate(yyyymmdd string) string {
	if len(yyyymmdd) != 8 {
		return ""
	}
	for _, r := range yyyymmdd {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return yyyymmdd[:4] + "-" + yyyymmdd[4:6] + "-" + yyyymmdd[6:]
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
