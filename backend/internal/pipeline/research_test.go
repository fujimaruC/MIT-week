package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/amba/amba/backend/internal/grounding"
	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
)

// fakeResearch answers every query with the same results (or error) and
// records what it was asked.
type fakeResearch struct {
	results []grounding.Result
	err     error
	asked   []string
}

func (f *fakeResearch) Enabled() bool { return true }
func (f *fakeResearch) Search(_ context.Context, q string) ([]grounding.Result, error) {
	f.asked = append(f.asked, q)
	return f.results, f.err
}

func researchPipeline(s grounding.Searcher) *Pipeline {
	return &Pipeline{
		Grounder:         grounding.New(grounding.DefaultAllowlist),
		Research:         s,
		ResearchGrounder: grounding.New(grounding.WebAllowlist),
	}
}

func TestResearchTermsPrefersKeywordsThenWhatTheVideoShows(t *testing.T) {
	vf := llm.VideoFindings{SearchQuery: "banjir jakarta mobil terseret"}
	if got := researchTerms(vf); got != "banjir jakarta mobil terseret" {
		t.Fatalf("keywords first: got %q", got)
	}
	vf = llm.VideoFindings{W5H1: llm.W5H1{What: "Banjir menyeret mobil", Where: "di Jakarta Utara", When: "Senin pagi"}}
	if got := researchTerms(vf); got != "Banjir menyeret mobil di Jakarta Utara Senin pagi" {
		t.Fatalf("5W1H next: got %q", got)
	}
	if got := researchTerms(llm.VideoFindings{W5H1: llm.W5H1{What: "Kucing"}}); got != "" {
		t.Fatalf("too thin to search: got %q", got)
	}
}

// The audit's own searches come first; with two pages or more no credit is spent.
func TestResearchReusesWhatTheAuditAlreadyFound(t *testing.T) {
	fake := &fakeResearch{}
	p := researchPipeline(fake)
	r := p.buildResearch(context.Background(), researchInput{
		terms:    "dasco usulan pemakzulan gibran",
		evidence: []models.WebEvidence{{Title: "Respon Dasco soal usulan pemakzulan", URL: "https://nasional.kompas.com/b", Publisher: "nasional.kompas.com", Says: llm.WebReal}},
		facts: []models.Source{
			{Title: "[SALAH] Syarat Pemakzulan Gibran Terpenuhi", URL: "https://turnbackhoax.id/x", Publisher: "turnbackhoax.id", Match: models.FactUnverified},
			{Title: "Respon Dasco soal usulan pemakzulan", URL: "https://nasional.kompas.com/b", Publisher: "nasional.kompas.com"},
			{Title: "Harga cabai naik di pasar", URL: "https://www.detik.com/z", Publisher: "detik.com", Match: models.FactUnverified},
		},
		pages: []grounding.Result{
			{Title: "Respon Dasco soal usulan pemakzulan", URL: "https://nasional.kompas.com/b?page=all", Publisher: "nasional.kompas.com"},
			{Title: "Dasco: tunggu masa sidang", URL: "https://news.okezone.com/c", Publisher: "news.okezone.com", Snippet: strings.Repeat("dasco ", 80)},
			{Title: "Blog acak", URL: "https://blog-acak.example/d", Publisher: "blog-acak.example"},
		},
	})
	if r == nil || r.Status != models.ResearchFound || len(r.Sources) != 3 {
		t.Fatalf("want 3 trusted pages, once each: %+v", r)
	}
	if len(fake.asked) != 0 || r.Query != "" {
		t.Fatalf("no new search when the audit found enough: asked %v, query %q", fake.asked, r.Query)
	}
	if r.Sources[0].Domain != "nasional.kompas.com" || r.Sources[1].Domain != "turnbackhoax.id" {
		t.Fatalf("judged pages first, then the claim check's: %+v", r.Sources)
	}
	if n := len([]rune(r.Sources[2].Snippet)); n > 280 {
		t.Fatalf("snippet capped at 280 runes, got %d", n)
	}
}

func TestResearchSearchesOnlyWhenTheAuditFoundTooLittle(t *testing.T) {
	fake := &fakeResearch{results: []grounding.Result{
		{Title: "Banjir rendam Jakarta Utara, mobil terseret arus", URL: "https://www.kompas.com/a", Publisher: "kompas.com", Snippet: "banjir jakarta"},
		{Title: "Banjir Jakarta mobil terseret", URL: "https://blog-acak.example/banjir", Publisher: "blog-acak.example", Snippet: "banjir jakarta"},
		{Title: "Harga cabai naik di pasar", URL: "https://www.detik.com/b", Publisher: "detik.com", Snippet: "cabai rawit"},
		{Title: "Video mobil terseret banjir di Jakarta", URL: "http://turnbackhoax.id/c", Publisher: "turnbackhoax.id", Snippet: "banjir jakarta"},
	}}
	r := researchPipeline(fake).buildResearch(context.Background(), researchInput{terms: "banjir jakarta mobil terseret"})
	if len(fake.asked) != 1 || fake.asked[0] != "banjir jakarta mobil terseret" || r.Query != "banjir jakarta mobil terseret" {
		t.Fatalf("one search with the video's terms: %v %q", fake.asked, r.Query)
	}
	if r.Status != models.ResearchFound || len(r.Sources) != 1 || r.Sources[0].Domain != "kompas.com" {
		t.Fatalf("only the trusted, on-topic https page: %+v", r)
	}
}

func TestResearchSaysPlainlyWhenItFoundNothingOrCouldNotRun(t *testing.T) {
	none := researchPipeline(&fakeResearch{}).buildResearch(context.Background(), researchInput{terms: "banjir jakarta"})
	if none == nil || none.Status != models.ResearchNone || none.Sources == nil || len(none.Sources) != 0 {
		t.Fatalf("nothing found: %+v", none)
	}
	down := researchPipeline(&fakeResearch{err: errors.New("432")}).buildResearch(context.Background(), researchInput{terms: "banjir jakarta"})
	if down.Status != models.ResearchUnavailable {
		t.Fatalf("a failed search is unavailable, not 'nothing found': %+v", down)
	}
	if r := (&Pipeline{}).buildResearch(context.Background(), researchInput{}); r != nil {
		t.Fatalf("nothing to show and nothing to search: %+v", r)
	}
}
