package pipeline

import (
	"context"
	"slices"
	"testing"

	"github.com/amba/amba/backend/internal/grounding"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/scoring"
	"github.com/amba/amba/backend/internal/video"
)

// metaOnlyClip is what the fetcher returns when the download failed but the
// post's metadata was read: no frames.
func metaOnlyClip(title, uploader string) *video.Clip {
	return &video.Clip{Platform: "youtube", Title: title, Uploader: uploader, DurationSec: 560, UploadDate: "20261109"}
}

// Case 4: a long compilation whose download failed. The metadata survives, so
// the creator's disclosure still runs and sets the AI provenance floor; the
// video itself is UNAVAILABLE with its reason.
func TestFailedDownloadStillReadsTheCreatorDisclosure(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	p.Video = &fakeVideo{clip: metaOnlyClip("Sora 2 AI Animal Videos Compilation", "Kanal Uji"), err: video.ErrDownload}

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	v := res.Video
	if v == nil || v.Status != models.VideoUnavailable || v.Reason != "download_failed" || len(v.Frames) != 0 {
		t.Fatalf("video = %+v", v)
	}
	if v.CreatorDisclosure.Level != models.DisclosureStrong {
		t.Fatalf("disclosure = %+v", v.CreatorDisclosure)
	}
	pv := res.Pillars.AI.Provenance
	if !res.Pillars.AI.Available || pv.Kind != models.ProvenanceAIGenerated || pv.Source != "creator" {
		t.Fatalf("AI pillar = %+v", res.Pillars.AI)
	}
	if verdict := Verdict(res, false); !slices.Contains(verdict.FloorsApplied, scoring.FloorAIProvenance) || verdict.Status == models.StatusInsufficient {
		t.Fatalf("verdict = %+v", verdict)
	}
	if !res.Degraded {
		t.Fatal("a transient download failure must not be cached")
	}
}

// Web evidence alone (no disclosure in the title) also sets the floor.
func TestFailedDownloadStillUsesTheWebProvenanceSearch(t *testing.T) {
	p, s, closeFn := webPipeline(t, []grounding.Result{
		{Title: "Kompilasi video hewan lucu viral hari ini ternyata buatan AI", Publisher: "kompas.com", URL: "https://www.kompas.com/cekfakta/read/9"},
	}, nil)
	defer closeFn()
	p.Video = &fakeVideo{clip: metaOnlyClip("Kompilasi video hewan lucu viral hari ini", "Kanal Uji"), err: video.ErrTimeout}

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	if len(s.queries) != 1 {
		t.Fatalf("web queries = %v", s.queries)
	}
	if pv := res.Pillars.AI.Provenance; pv.Kind != models.ProvenanceAIGenerated || pv.Source != "web" {
		t.Fatalf("provenance = %+v", pv)
	}
	if res.Video.Status != models.VideoUnavailable || res.Video.Reason != "timeout" || len(res.Video.WebEvidence) != 1 {
		t.Fatalf("video = %+v", res.Video)
	}
	if verdict := Verdict(res, false); !slices.Contains(verdict.FloorsApplied, scoring.FloorAIProvenance) {
		t.Fatalf("verdict = %+v", verdict)
	}
}

// With no frames and no answer from the model, the title is the claim: the
// fact check still runs and can find the debunk.
func TestTitleIsTheClaimWhenTheModelNeverRan(t *testing.T) {
	m := &scriptedModel{failOther: true, supports: contradictsDebunk}
	p, _, site := hoaxPipeline(t, m, nil,
		[]grounding.Result{{Title: "[SALAH] Purbaya Dilantik Jadi Gubernur Bank Indonesia", Publisher: "turnbackhoax.id",
			URL: "https://turnbackhoax.id/articles/36857-salah-purbaya-dilantik-jadi-gubernur-bank-indonesia", Rating: "[SALAH]"}},
		metaOnlyClip("Detik-detik Purbaya dilantik jadi Gubernur Bank Indonesia", "akun biasa"))
	p.Video = &fakeVideo{clip: metaOnlyClip("Detik-detik Purbaya dilantik jadi Gubernur Bank Indonesia", "akun biasa"), err: video.ErrDownload}

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	fact := res.Pillars.Fact
	if fact.Claim != "Detik-detik Purbaya dilantik jadi Gubernur Bank Indonesia" || fact.Status != models.FactContradicts {
		t.Fatalf("fact = %+v", fact)
	}
	if len(site.queries) == 0 {
		t.Fatal("the fact-checker's own search never ran")
	}
	if res.Video.Status != models.VideoUnavailable || !res.Degraded {
		t.Fatalf("video = %+v degraded=%v", res.Video, res.Degraded)
	}
	if verdict := Verdict(res, false); verdict.Status != models.StatusTampered {
		t.Fatalf("verdict = %+v", verdict)
	}
}

// A retry after a stalled download keeps the metadata of the first try.
func TestRetryKeepsMetadataWhenTheSecondTryHasNone(t *testing.T) {
	p, srv := pipelineWithFetch(t, true, ogPage)
	defer srv.Close()
	fv := &fakeVideoSeq{steps: []fetchStep{
		{clip: metaOnlyClip("Sora 2 AI Animal Videos Compilation", "Kanal Uji"), err: video.ErrTimeout},
		{err: video.ErrDownload},
	}}
	p.Video = fv
	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	if res.Video.CreatorDisclosure.Level != models.DisclosureStrong || res.Video.Reason != "download_failed" {
		t.Fatalf("video = %+v", res.Video)
	}
}

type fetchStep struct {
	clip *video.Clip
	err  error
}

type fakeVideoSeq struct {
	steps []fetchStep
	i     int
}

func (f *fakeVideoSeq) Enabled() bool { return true }
func (f *fakeVideoSeq) Fetch(context.Context, string) (*video.Clip, error) {
	s := f.steps[min(f.i, len(f.steps)-1)]
	f.i++
	return s.clip, s.err
}
