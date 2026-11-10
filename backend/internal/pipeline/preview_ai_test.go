package pipeline

import (
	"slices"
	"testing"

	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/scoring"
)

// A video link whose footage could not be downloaded is judged from its
// preview picture: one that looks plainly AI-made (the photo bar) is a reason
// to check, below it nothing changes (10 Nov: an Instagram reel the labelled
// set calls AI scored 95 on its preview and came out SAFE).
func TestAnAIMadeLookingPreviewOfAnUndownloadableVideoIsAReasonToCheck(t *testing.T) {
	res := Result{InputType: models.InputURL, Video: &models.VideoAnalysis{Status: models.VideoUnavailable, Reason: "download_failed"}}
	res.Pillars.AI = models.AIPillar{Available: true, Score: 95, Reliability: 0.6}

	v := Verdict(res, false)
	if v.Status != models.StatusSuspicious || !slices.Contains(v.FloorsApplied, scoring.FloorAILikely) {
		t.Fatalf("preview AI 95 on an undownloadable video: want SUSPICIOUS with the AI-likely floor, got %+v", v)
	}

	res.Pillars.AI.Score = scoring.AILikelyMin - 1
	if v := Verdict(res, false); slices.Contains(v.FloorsApplied, scoring.FloorAILikely) {
		t.Fatalf("below the photo bar the preview changes nothing: %+v", v)
	}

	// A downloaded video keeps its own threshold and is not touched by the rule.
	res.Video.Status, res.Pillars.AI.Score = models.VideoOK, 95
	if v := Verdict(res, false); v.Status == models.StatusSafe {
		t.Fatalf("a downloaded AI video is flagged by its own floor: %+v", v)
	}
}
