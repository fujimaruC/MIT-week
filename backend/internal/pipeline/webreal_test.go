package pipeline

import (
	"testing"

	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
)

func contradicted() models.FactPillar {
	return models.FactPillar{Available: true, Claim: "Pimpinan DPR belum membahas usulan pemakzulan", Status: models.FactContradicts, Sources: []models.Source{
		{Title: "[SALAH] Syarat Pemakzulan Terpenuhi, Gibran Pulang ke Solo", Publisher: "turnbackhoax.id", URL: "https://turnbackhoax.id/x", Match: models.FactContradicts},
		{Title: "Ramai-ramai bersuara soal pemakzulan", Publisher: "cnnindonesia.com", URL: "https://www.cnnindonesia.com/y", Match: models.FactUnverified},
	}}
}

func TestTwoNewsroomsReportingTheVideoAsRealOutweighOneDebunkOfAnotherClaim(t *testing.T) {
	w := webOutcome{Evidence: []models.WebEvidence{
		{Title: "Dasco soal usulan pemakzulan Gibran", URL: "https://news.okezone.com/a", Publisher: "news.okezone.com", Says: llm.WebReal},
		{Title: "Respon Dasco soal surat usulan pemakzulan", URL: "https://nasional.kompas.com/b", Publisher: "nasional.kompas.com", Says: llm.WebReal},
	}}
	got := weighWebReal(contradicted(), w)
	if got.Status != models.FactUnverified {
		t.Fatalf("want UNVERIFIED, got %s", got.Status)
	}
	if got.Sources[0].Match != models.FactUnverified || len(got.Sources) != 2 {
		t.Fatalf("the debunk is kept, no longer as a contradiction: %+v", got.Sources)
	}
	if contradicted().Sources[0].Match != models.FactContradicts {
		t.Fatal("the input is not modified")
	}
}

func TestADebunkStandsWhenAnyPageCallsTheVideoAHoaxOrOnlyOneNewsroomSaysReal(t *testing.T) {
	hoax := webOutcome{Evidence: []models.WebEvidence{
		{Title: "a", URL: "https://news.okezone.com/a", Publisher: "news.okezone.com", Says: llm.WebReal},
		{Title: "b", URL: "https://nasional.kompas.com/b", Publisher: "nasional.kompas.com", Says: llm.WebReal},
		{Title: "[HOAKS] video lama", URL: "https://turnbackhoax.id/c", Publisher: "turnbackhoax.id", Says: llm.WebHoax},
	}}
	if got := weighWebReal(contradicted(), hoax); got.Status != models.FactContradicts {
		t.Fatalf("a page calling the video a hoax keeps the contradiction, got %s", got.Status)
	}
	same := webOutcome{Evidence: []models.WebEvidence{
		{Title: "a", URL: "https://nasional.kompas.com/a", Publisher: "nasional.kompas.com", Says: llm.WebReal},
		{Title: "a2", URL: "https://nasional.kompas.com/a2", Publisher: "nasional.kompas.com", Says: llm.WebReal},
	}}
	if got := weighWebReal(contradicted(), same); got.Status != models.FactContradicts {
		t.Fatalf("one newsroom twice is one newsroom, got %s", got.Status)
	}
}

func TestOnSubjectListsAPageOnce(t *testing.T) {
	ev := []models.WebEvidence{
		{Title: "Respon Dasco soal surat usulan pemakzulan Gibran", URL: "https://nasional.kompas.com/b", Publisher: "nasional.kompas.com", Says: llm.WebReal},
		{Title: "Respon Dasco soal surat usulan pemakzulan Gibran", URL: "https://nasional.kompas.com/b", Publisher: "nasional.kompas.com", Says: llm.WebReal},
	}
	if got := onSubject(ev, "Dasco usulan pemakzulan Gibran"); len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
}
