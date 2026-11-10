package pipeline

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/scoring"
	"github.com/amba/amba/backend/internal/video"
)

const tiktokURL = "https://www.tiktok.com/@akun/video/7300000000000000000"

// fakeVideo stands in for yt-dlp and ffmpeg. firstErr, when set, is
// returned by the first call only.
type fakeVideo struct {
	clip     *video.Clip
	err      error
	firstErr error
	calls    int32
}

func (f *fakeVideo) Enabled() bool { return true }

func (f *fakeVideo) Fetch(context.Context, string) (*video.Clip, error) {
	if atomic.AddInt32(&f.calls, 1) == 1 && f.firstErr != nil {
		return nil, f.firstErr
	}
	return f.clip, f.err
}

func TestStalledDownloadIsRetriedOnce(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	fv := &fakeVideo{clip: testClip(t), firstErr: video.ErrTimeout}
	p.Video = fv

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	if res.Video == nil || res.Video.Status != models.VideoOK || atomic.LoadInt32(&fv.calls) != 2 {
		t.Fatalf("video %+v after %d fetches", res.Video, fv.calls)
	}
}

func testClip(t *testing.T) *video.Clip {
	return &video.Clip{
		Platform: "tiktok", Title: "Banjir besar hari ini", Description: "Segera sebarkan", UploadDate: "20261109",
		DurationSec: 31.44,
		Frames: []video.Frame{
			{AtSec: 1.52, JPEG: testJPEG(t, 1)}, {AtSec: 15, JPEG: testJPEG(t, 2)}, {AtSec: 29.9, JPEG: testJPEG(t, 3)},
		},
	}
}

func ogPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<html><head><meta property="og:title" content="Banjir besar hari ini"></head></html>`))
}

func TestVideoLinkIsJudgedOnItsFrames(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	fv := &fakeVideo{clip: testClip(t)}
	p.Video = fv

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	v := res.Video
	if v == nil || v.Status != models.VideoOK || v.Reason != "" || v.Platform != "tiktok" ||
		v.UploadDate != "2026-11-09" || v.DurationSec != 31.4 {
		t.Fatalf("video = %+v", v)
	}
	if len(v.Frames) != 3 || v.Frames[0].AtSec != 1.5 || !strings.HasPrefix(v.Frames[2].Thumb, "data:image/jpeg;base64,") {
		t.Fatalf("frames = %+v", v.Frames)
	}
	if v.Summary == "" || !res.Pillars.AI.Available {
		t.Fatalf("summary %q, AI pillar %+v", v.Summary, res.Pillars.AI)
	}
	// Whatever the mock found, the floors must follow the video's findings.
	floors := Verdict(res, false).FloorsApplied
	if (res.Pillars.AI.Score >= scoring.VideoAILikelyMin) != slices.Contains(floors, scoring.FloorAILikely) {
		t.Fatalf("AI score %d, floors %v", res.Pillars.AI.Score, floors)
	}
	if (v.Context.Score >= scoring.VideoContextMin) != slices.Contains(floors, scoring.FloorVideoContextMismatch) {
		t.Fatalf("context %d, floors %v", v.Context.Score, floors)
	}
}

func TestPlatformAILabelIsProvenance(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	clip := testClip(t)
	clip.AILabel = true
	p.Video = &fakeVideo{clip: clip}

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	if !res.Video.PlatformAILabel || res.Pillars.AI.Provenance.Kind != models.ProvenanceAIGenerated {
		t.Fatalf("video %+v, provenance %+v", res.Video, res.Pillars.AI.Provenance)
	}
	if v := Verdict(res, false); !slices.Contains(v.FloorsApplied, scoring.FloorAIProvenance) || v.Status != models.StatusTampered {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestPrivateVideoFallsBackToThePreview(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	fv := &fakeVideo{err: video.ErrPrivate}
	p.Video = fv

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	if res.Video == nil || res.Video.Status != models.VideoUnavailable || res.Video.Reason != "private" || len(res.Video.Frames) != 0 {
		t.Fatalf("video = %+v", res.Video)
	}
	if slices.Contains(Verdict(res, false).FloorsApplied, scoring.FloorAILikely) {
		t.Fatal("an unavailable video must not be judged like one")
	}
}

func TestTransientVideoFailureIsNotCached(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	fv := &fakeVideo{err: video.ErrTimeout}
	p.Video = fv

	for i := 0; i < 2; i++ {
		res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
		if res.Video == nil || res.Video.Reason != "timeout" || !res.Degraded {
			t.Fatalf("run %d: video %+v degraded %v", i, res.Video, res.Degraded)
		}
	}
	// Each run tries twice (one retry after a stall); nothing is cached.
	if n := atomic.LoadInt32(&fv.calls); n != 4 {
		t.Fatalf("fetched %d times, want 4 (two runs, one retry each, nothing cached)", n)
	}
}

func TestOtherLinksAreNotDownloaded(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	fv := &fakeVideo{clip: testClip(t)}
	p.Video = fv

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: "https://berita.example/banjir"})
	if res.Video != nil || atomic.LoadInt32(&fv.calls) != 0 {
		t.Fatalf("video = %+v, fetches = %d", res.Video, fv.calls)
	}
}

func TestVideoReasonCodes(t *testing.T) {
	for _, tc := range []struct {
		err       error
		reason    string
		transient bool
	}{
		{video.ErrUnsupported, "unsupported", false},
		{video.ErrPrivate, "private", false},
		{video.ErrTooLong, "too_long", false},
		{video.ErrTooBig, "too_big", false},
		{video.ErrTimeout, "timeout", true},
		{context.DeadlineExceeded, "timeout", true},
		{video.ErrNoTools, "no_tools", true},
		{errors.New("anything else"), "download_failed", true},
	} {
		if r, tr := videoReason(tc.err); r != tc.reason || tr != tc.transient {
			t.Errorf("%v: got %q/%v, want %q/%v", tc.err, r, tr, tc.reason, tc.transient)
		}
	}
	if isoDate("20261109") != "2026-11-09" || isoDate("2026110") != "" || isoDate("2026-11-") != "" {
		t.Fatal("isoDate")
	}
}

func TestStrongCreatorDisclosureIsProvenance(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	clip := testClip(t)
	clip.Title = "Epic Fan-Made AI Creation #aivideo"
	clip.Uploader = "HaiperAI"
	p.Video = &fakeVideo{clip: clip}

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	if d := res.Video.CreatorDisclosure; d.Level != models.DisclosureStrong || d.Phrase == "" {
		t.Fatalf("disclosure = %+v", d)
	}
	pv := res.Pillars.AI.Provenance
	if pv.Kind != models.ProvenanceAIGenerated || pv.Source != "creator" || pv.Generator != "Haiper" {
		t.Fatalf("provenance = %+v", pv)
	}
	if v := Verdict(res, false); !slices.Contains(v.FloorsApplied, scoring.FloorAIProvenance) {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestPlatformLabelOutranksCreatorDisclosure(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	clip := testClip(t)
	clip.AILabel, clip.Title = true, "Dibuat dengan AI"
	p.Video = &fakeVideo{clip: clip}
	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	if res.Pillars.AI.Provenance.Source != "watermark" {
		t.Fatalf("provenance = %+v", res.Pillars.AI.Provenance)
	}
}

func TestWeakDisclosureOnlyBoostsAndNewsAboutAIIsIgnored(t *testing.T) {
	level := func(title, uploader string) string {
		p, srv := pipelineWithFetch(t, true, ogPage)
		defer srv.Close()
		clip := testClip(t)
		clip.Title, clip.Uploader = title, uploader
		p.Video = &fakeVideo{clip: clip}
		res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
		if res.Pillars.AI.Provenance.Kind == models.ProvenanceAIGenerated {
			return "provenance"
		}
		return res.Video.CreatorDisclosure.Level
	}
	if got := level("Banjir besar hari ini", "akun"); got != models.DisclosureNone {
		t.Fatalf("plain: %s", got)
	}
	if got := level("Banjir besar hari ini #ai", "akun"); got != models.DisclosureWeak {
		t.Fatalf("weak: %s", got)
	}
	if got := level("Waspada video AI palsu, ini aturannya", "Kompas.com"); got != models.DisclosureNone {
		t.Fatalf("news about AI: %s", got)
	}
}

func TestWeakDisclosureAddsASmallBoost(t *testing.T) {
	ev := aiEvidence{modelOK: true, findings: llm.Findings{AIScore: 30}}
	base, _ := buildAIPillar(ev)
	ev.weakDisclosure = true
	boosted, _ := buildAIPillar(ev)
	if boosted.Score != base.Score+weakDisclosureBoost {
		t.Fatalf("boosted %d, base %d", boosted.Score, base.Score)
	}
	ev.findings.AIScore = 98
	if top, _ := buildAIPillar(ev); top.Score != 100 {
		t.Fatalf("score %d, want it capped at 100", top.Score)
	}
}
