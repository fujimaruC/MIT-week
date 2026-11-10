package pipeline

import (
	"slices"
	"strings"
	"testing"

	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/scoring"
)

// An unverified claim is shown but is not evidence: on 9 Nov it made all
// 11 real Kompas news videos and 16 of 20 untouched price graphics SUSPICIOUS.
func TestUnverifiedClaimIsNotRisk(t *testing.T) {
	unverified := Result{InputType: models.InputURL, Pillars: models.Pillars{
		Fact: models.FactPillar{Available: true, Claim: "Gedung runtuh hari ini", Status: models.FactUnverified},
		Link: models.LinkPillar{Available: true, Score: 0},
	}}
	v := Verdict(unverified, false)
	if v.Status != models.StatusSafe || v.Score == nil || *v.Score > scoring.SafeMax {
		t.Fatalf("unverified claim: status %s score %v, want SAFE", v.Status, v.Score)
	}
	if len(v.NotChecked) != 0 {
		t.Fatalf("not_checked = %v, nothing applicable failed", v.NotChecked)
	}

	contradicted := unverified
	contradicted.Pillars.Fact = models.FactPillar{Available: true, Claim: "Gedung runtuh hari ini", Status: models.FactContradicts,
		Sources: []models.Source{{URL: "https://turnbackhoax.id/x", Match: models.FactContradicts}}}
	if v := Verdict(contradicted, false); !slices.Contains(v.FloorsApplied, scoring.FloorFactContradicted) {
		t.Fatalf("contradicted claim: %+v", v)
	}
}

// not_checked lists only the checks that apply to the input and failed.
func TestNotCheckedListsOnlyApplicableFailures(t *testing.T) {
	link := Result{InputType: models.InputURL, Pillars: models.Pillars{Link: models.LinkPillar{Available: true}}}
	if v := Verdict(link, false); slices.Contains(v.NotChecked, "ela") || len(v.NotChecked) != 0 {
		t.Fatalf("link: not_checked = %v, ELA does not apply to a link", v.NotChecked)
	}
	up := Result{InputType: models.InputFile, Failed: []string{"ela"}}
	if v := Verdict(up, false); !slices.Equal(v.NotChecked, []string{"ela"}) {
		t.Fatalf("upload whose ELA failed: not_checked = %v", v.NotChecked)
	}
	// A failure is listed only while the pillar has no result.
	ok := Result{InputType: models.InputFile, Failed: []string{"ela"}, Pillars: models.Pillars{ELA: models.ELAPillar{Available: true}}}
	if v := Verdict(ok, false); len(v.NotChecked) != 0 {
		t.Fatalf("not_checked = %v", v.NotChecked)
	}
}

func TestSafeLineIsClearAndFitsTheContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		res  Result
		want string
	}{
		{"unreachable video", Result{InputType: models.InputURL,
			Video:   &models.VideoAnalysis{Status: models.VideoUnavailable},
			Pillars: models.Pillars{AI: models.AIPillar{Available: true}, Fact: models.FactPillar{Available: true, Status: models.FactUnverified}}},
			"Tidak ditemukan tanda AI atau bantahan cek fakta. Penilaian memakai gambar pratinjau dan pencarian cek fakta."},
		{"no article yet", Result{InputType: models.InputURL, Pillars: models.Pillars{Fact: models.FactPillar{Available: true, Status: models.FactUnverified}}},
			"Tidak ditemukan bantahan cek fakta. Belum ada artikel cek fakta yang membantah atau membenarkan klaim ini."},
		{"no claim in an image", Result{InputType: models.InputFile, Pillars: models.Pillars{AI: models.AIPillar{Available: true}}},
			"Tidak ditemukan tanda AI. Gambar ini tidak memuat klaim yang bisa dicek."},
	} {
		got := safeSimple(tc.res)
		if got != tc.want || len([]rune(got)) > 140 {
			t.Errorf("%s: %q (%d)", tc.name, got, len([]rune(got)))
		}
		for _, bad := range []string{"Tidak dapat diperiksa", "tidak dapat diperiksa", "Belum cukup"} {
			if strings.Contains(got, bad) {
				t.Errorf("%s: %q", tc.name, got)
			}
		}
	}
}

func TestPanicInAPillarIsContained(t *testing.T) {
	if err := guard(func() error { panic("boom") })(); err != nil {
		t.Fatalf("err = %v", err)
	}
	ran := false
	if err := guard(func() error { ran = true; return nil })(); err != nil || !ran {
		t.Fatal("a normal function must run")
	}
}

func TestAPillarThatDidNotFinishIsNeverReportedAsFailed(t *testing.T) {
	var seen []Event
	tr := newTracker(func(e Event) { seen = append(seen, e) })
	tr.start(PillarFact)
	tr.fail(PillarFact)
	tr.finish()
	for _, e := range seen {
		if e.Status == "failed" {
			t.Fatalf("error event: %+v", e)
		}
	}
}
