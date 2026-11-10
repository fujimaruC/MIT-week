package pipeline

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/amba/amba/backend/internal/grounding"
	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/scoring"
	"github.com/amba/amba/backend/internal/video"
)

// scriptedModel stands in for the model gateway. It answers the video call
// with findings, and the claim classification by a rule on each result's
// title, and keeps the classification request for the test to read.
type scriptedModel struct {
	findings map[string]any
	supports func(title string) string // "CONTRADICTS", "SUPPORTS" or ""
	// failOther makes every call except the claim classification fail: the
	// model never answers about the content itself.
	failOther  bool
	mu         sync.Mutex
	classified []string
}

func (m *scriptedModel) serve(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		if m.failOther && !strings.Contains(body, "ClaimMatches") {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		var inner string
		switch {
		case strings.Contains(body, "ClaimMatches"):
			m.mu.Lock()
			m.classified = append(m.classified, body)
			m.mu.Unlock()
			var matches []map[string]any
			for i := 0; ; i++ {
				// Results are numbered "[i] Judul: ..." in the user message.
				marker := `[` + string(rune('0'+i)) + `] Judul: `
				at := strings.Index(body, marker)
				if at < 0 {
					break
				}
				line := body[at+len(marker):]
				if end := strings.Index(line, " | Penerbit"); end >= 0 {
					line = line[:end]
				}
				if verdict := m.supports(line); verdict != "" {
					matches = append(matches, map[string]any{"index": i, "match": verdict})
				}
			}
			b, _ := json.Marshal(map[string]any{"matches": matches})
			inner = string(b)
		case strings.Contains(body, "VideoFindings"):
			b, _ := json.Marshal(m.findings)
			inner = string(b)
		default:
			// The web provenance classification: nothing found.
			inner = `{"matches":[]}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": inner}}},
		})
	}))
}

func videoFindingsMap(claim, summary string, aiScore int) map[string]any {
	return map[string]any{
		"summary": summary, "transcript": "", "claim": claim, "search_query": "",
		"ai_score": aiScore, "ai_indicators": []string{}, "ai_watermark": "",
		"manipulation_score": 0, "manipulation_indicators": []string{},
		"context_mismatch_score": 0, "context_indicators": []string{},
		"ocr_text": "", "tactics": []any{}, "injection_suspected": false,
	}
}

// hoaxPipeline builds a pipeline whose model is scripted and whose two fact
// searches are fakes.
func hoaxPipeline(t *testing.T, m *scriptedModel, general, site []grounding.Result, clip *video.Clip) (*Pipeline, *fakeSearcher, *fakeSearcher) {
	t.Helper()
	gw := m.serve(t)
	t.Cleanup(gw.Close)
	p, srv := pipelineWithFetch(t, false, ogPage)
	t.Cleanup(srv.Close)
	p.LLM = llm.New(gw.URL, "k", "m", false)
	g, s := &fakeSearcher{results: general}, &fakeSearcher{results: site}
	p.Searcher, p.SiteSearch = g, s
	p.Video = &fakeVideo{clip: clip}
	return p, g, s
}

func clipTitled(t *testing.T, title, uploader string) *video.Clip {
	c := testClip(t)
	c.Title, c.Description, c.Uploader = title, "", uploader
	return c
}

func contradictsDebunk(title string) string {
	if strings.HasPrefix(title, "[HOAKS]") || strings.HasPrefix(title, "[SALAH]") {
		return "CONTRADICTS"
	}
	return ""
}

// Case 1: the claim is descriptive, the fact-check debunks the implied
// narrative. The classifier now sees the title and the summary, and the
// source stays on topic because the content words count.
func TestImpliedNarrativeIsContradicted(t *testing.T) {
	m := &scriptedModel{
		findings: videoFindingsMap("Purbaya menghadiri peringatan HUT Bea Cukai dengan seragam Bea Cukai",
			"Seorang pria berseragam Bea Cukai berpidato di acara dinas.", 5),
		supports: contradictsDebunk,
	}
	p, _, _ := hoaxPipeline(t, m,
		[]grounding.Result{{Title: "Profil Purbaya", Publisher: "kompas.com", URL: "https://www.kompas.com/tag/purbaya"}},
		[]grounding.Result{{Title: "[HOAKS] Purbaya Ditunjuk Menjadi Ketua Bea Cukai", Publisher: "kompas.com", URL: "https://www.kompas.com/cekfakta/read/2026/11/10/purbaya-ketua-bea-cukai", Rating: "[HOAKS]"}},
		clipTitled(t, "Purbaya jadi bos baru Bea Cukai? Seragam lengkap!", "akun biasa"))

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	if res.Pillars.Fact.Status != models.FactContradicts {
		t.Fatalf("fact = %+v", res.Pillars.Fact)
	}
	if v := Verdict(res, false); v.Status != models.StatusTampered || !slices.Contains(v.FloorsApplied, scoring.FloorFactContradicted) {
		t.Fatalf("verdict = %+v", v)
	}
	if len(m.classified) != 1 {
		t.Fatalf("classifier calls = %d", len(m.classified))
	}
	req := m.classified[0]
	for _, want := range []string{"Purbaya jadi bos baru Bea Cukai", "Seorang pria berseragam Bea Cukai berpidato"} {
		if !strings.Contains(req, want) {
			t.Errorf("classifier request lacks %q", want)
		}
	}
}

// Case 2: Tavily only returns listing pages; the fact-checker's own search
// finds the debunk. The claim says "sumpah", the debunk says "dilantik", so
// only the title and the claim together tie the debunk to this content.
func TestDebunkFoundByTheFactCheckersOwnSearch(t *testing.T) {
	m := &scriptedModel{
		findings: videoFindingsMap("Purbaya mengucap sumpah jabatan di hadapan hakim agung",
			"Seorang pria mengangkat tangan kanan saat sumpah jabatan.", 5),
		supports: contradictsDebunk,
	}
	p, general, site := hoaxPipeline(t, m,
		[]grounding.Result{
			{Title: "Berita terbaru Purbaya", Publisher: "detik.com", URL: "https://www.detik.com/tag/purbaya"},
			{Title: "Indeks Bank Indonesia", Publisher: "kompas.com", URL: "https://www.kompas.com/tag/bank-indonesia"},
		},
		[]grounding.Result{{Title: "[SALAH] Purbaya Dilantik Jadi Gubernur Bank Indonesia", Publisher: "turnbackhoax.id",
			URL: "https://turnbackhoax.id/articles/36857-salah-purbaya-dilantik-jadi-gubernur-bank-indonesia", Rating: "[SALAH]",
			Snippet: "Konteks asli dokumentasi adalah momen Purbaya (kala itu masih Ketua LPS) mengucap sumpah."}},
		clipTitled(t, "Detik-detik Purbaya dilantik jadi Gubernur Bank Indonesia", "akun biasa"))

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	fact := res.Pillars.Fact
	if fact.Status != models.FactContradicts {
		t.Fatalf("fact = %+v", fact)
	}
	found := false
	for _, s := range fact.Sources {
		if strings.Contains(s.URL, "turnbackhoax.id/articles/36857") && s.Match == models.FactContradicts {
			found = true
		}
	}
	if !found {
		t.Fatalf("sources = %+v", fact.Sources)
	}
	if len(general.queries) != 1 {
		t.Fatalf("general queries = %v", general.queries)
	}
	if len(site.queries) == 0 || len(site.queries) > 3 {
		t.Fatalf("site queries = %v", site.queries)
	}
	for _, q := range site.queries {
		if n := len(strings.Fields(q)); n == 0 || n > 5 || !strings.Contains(q, "purbaya") {
			t.Errorf("site query %q: %d words", q, n)
		}
	}
	if !slices.ContainsFunc(site.queries, func(q string) bool { return strings.Contains(q, "gubernur") }) {
		t.Errorf("the title's own words are searched: %v", site.queries)
	}
}

// Case 2b: a different event that only shares the names stays UNVERIFIED even
// when the classifier is too generous.
func TestAnotherEventWithTheSameNamesStaysUnverified(t *testing.T) {
	m := &scriptedModel{
		findings: videoFindingsMap("Massa demo menuntut Gibran mundur", "Massa membawa spanduk di depan gedung.", 5),
		supports: func(string) string { return "CONTRADICTS" },
	}
	p, _, _ := hoaxPipeline(t, m, nil,
		[]grounding.Result{{Title: "[HOAKS] Pasukan PBB tiba di Aceh", Publisher: "turnbackhoax.id", URL: "https://turnbackhoax.id/articles/1-salah-pbb"}},
		clipTitled(t, "Demo di depan gedung", "akun biasa"))
	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	if res.Pillars.Fact.Status != models.FactUnverified {
		t.Fatalf("fact = %+v", res.Pillars.Fact)
	}
}

func lewotobiNews() []grounding.Result {
	return []grounding.Result{
		{Title: "Gunung Lewotobi Laki-laki erupsi lagi, kolom abu 2 km", Publisher: "detik.com", URL: "https://news.detik.com/berita/d-1/lewotobi-erupsi"},
		{Title: "Erupsi Gunung Lewotobi, warga mengungsi", Publisher: "antaranews.com", URL: "https://www.antaranews.com/berita/1/lewotobi"},
	}
}

func supportsNews(string) string { return "SUPPORTS" }

// Case 3: news says the volcano erupted. That does not say that THIS footage
// is from that event, so on a video it is shown but cannot make it SAFE.
func TestNewsSupportCannotMakeAVideoSafe(t *testing.T) {
	m := &scriptedModel{findings: videoFindingsMap("Gunung Lewotobi erupsi", "Letusan gunung api di malam hari.", 5), supports: supportsNews}
	p, _, _ := hoaxPipeline(t, m, lewotobiNews(), nil, clipTitled(t, "Erupsi Gunung Lewotobi malam ini", "akun biasa"))

	res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
	fact := res.Pillars.Fact
	if fact.Status != models.FactUnverified {
		t.Fatalf("status = %s, want UNVERIFIED", fact.Status)
	}
	shown := 0
	for _, s := range fact.Sources {
		if s.Match == models.FactSupports {
			shown++
		}
	}
	if shown != 2 {
		t.Fatalf("support is still shown: %+v", fact.Sources)
	}
	// The audit still concludes, and the line never claims the news supports it.
	v := Verdict(res, false)
	if v.Status != models.StatusSafe || strings.Contains(v.ExplanationSimple, "sejalan") {
		t.Fatalf("verdict = %+v", v)
	}
}

// The same support counts when the video comes from the outlet's own account,
// and for anything that is not a video.
func TestNewsSupportCountsForTheOfficialAccount(t *testing.T) {
	for _, tc := range []struct {
		uploader string
		want     models.FactStatus
	}{
		{"KOMPASTV", models.FactSupports},
		{"CNN Indonesia", models.FactSupports},
		{"kompastv_fans", models.FactUnverified},
		{"akun biasa", models.FactUnverified},
	} {
		m := &scriptedModel{findings: videoFindingsMap("Gunung Lewotobi erupsi", "Letusan gunung api.", 5), supports: supportsNews}
		p, _, _ := hoaxPipeline(t, m, lewotobiNews(), nil, clipTitled(t, "Erupsi Gunung Lewotobi malam ini", tc.uploader))
		res := p.Run(context.Background(), Request{InputType: models.InputURL, URL: tiktokURL})
		if got := res.Pillars.Fact.Status; got != tc.want {
			t.Errorf("%q: status = %s, want %s", tc.uploader, got, tc.want)
		}
		v := Verdict(res, false)
		if v.Status != models.StatusSafe {
			t.Errorf("%q: status = %s, every unflagged audit is SAFE", tc.uploader, v.Status)
		}
		agrees := strings.Contains(v.ExplanationSimple, "sejalan")
		if agrees != (tc.want == models.FactSupports) {
			t.Errorf("%q: line = %q with status %s", tc.uploader, v.ExplanationSimple, res.Pillars.Fact.Status)
		}
	}
}

func TestOfficialOutletIsAnExactHandle(t *testing.T) {
	for _, ok := range []string{"kompascom", "@kompastv", "CNN Indonesia", "cnnindonesiaofficial", "detikcom", "tempodotco",
		"MetroTVNews", "liputan6", "narasinewsroom", "BBC Indonesia", "antaranews", "Kompas.com", "@KompasTV"} {
		if !officialOutlet(ok) {
			t.Errorf("%q should be official", ok)
		}
	}
	for _, no := range []string{"", "kompas", "kompastv2", "kompas tv fans", "detikcom_official", "cnn", "kompas-tv-palsu", "kоmpastv"} {
		if officialOutlet(no) {
			t.Errorf("%q must not be official", no)
		}
	}
	if !officialOutlet("unknown", "@detikcom") || officialOutlet("unknown", "other") {
		t.Error("any of the names may match")
	}
}

// A contradiction from a fact-check article beats support from general news;
// only a fact-checker's own support can disagree with it.
func TestFactCheckContradictionBeatsNewsSupport(t *testing.T) {
	c := models.Source{Title: "[SALAH] Klaim X", URL: "https://turnbackhoax.id/articles/1-salah-x", Match: models.FactContradicts}
	news := models.Source{Title: "X terjadi", URL: "https://news.detik.com/x", Match: models.FactSupports}
	checked := models.Source{Title: "[BENAR] Klaim X", URL: "https://turnbackhoax.id/articles/2-benar-x", Match: models.FactSupports}
	if got := statusFromSources([]models.Source{news, c}); got != models.FactContradicts {
		t.Fatalf("news + debunk = %s", got)
	}
	if got := statusFromSources([]models.Source{checked, c}); got != models.FactUnverified {
		t.Fatalf("fact-checkers disagree = %s", got)
	}
	if got := factStatus([]models.Source{news, c}, factContext{Video: true}); got != models.FactContradicts {
		t.Fatalf("video: news + debunk = %s", got)
	}
	if got := factStatus([]models.Source{news}, factContext{Video: true}); got != models.FactUnverified {
		t.Fatalf("video: news only = %s", got)
	}
	if got := factStatus([]models.Source{checked}, factContext{Video: true}); got != models.FactSupports {
		t.Fatalf("video: a fact-checker's support counts = %s", got)
	}
	if got := factStatus([]models.Source{news}, factContext{}); got != models.FactSupports {
		t.Fatalf("image: news support unchanged = %s", got)
	}
}

// TurnBackHoax needs every query word in a title: live, "purbaya didampingi
// sumpah gubernur bank" found nothing while "purbaya gubernur" found the
// debunk. Queries stay at two or three words, at most three of them.
func TestFactQueriesAreShortAndDistinct(t *testing.T) {
	qs := factQueries("Purbaya menghadiri peringatan HUT Bea Cukai dengan seragam", factContext{Title: "Purbaya jadi bos baru Bea Cukai? #fyp"})
	if want := []string{"purbaya hut", "purbaya bea", "purbaya hut bea"}; strings.Join(qs, "|") != strings.Join(want, "|") {
		t.Fatalf("queries = %q, want %q", qs, want)
	}
	for _, q := range qs {
		if len(strings.Fields(q)) > 3 {
			t.Errorf("query %q has more than three words", q)
		}
	}
	bi := factQueries("Purbaya didampingi saat menghadiri acara pembacaan sumpah Gubernur Bank Indonesia.",
		factContext{Title: "Mendampingi Pak Purbaya di Acara Pembacaan Sumpah Gubernur Bank Indonesia"})
	if len(bi) == 0 || bi[0] != "purbaya gubernur" {
		t.Fatalf("the Gubernur BI case must lead with the query that finds the debunk: %q", bi)
	}
	for _, q := range bi {
		if strings.Contains(q, "pak") {
			t.Errorf("honorific in query %q", q)
		}
	}
	if got := factQueries("Gunung Lewotobi erupsi", factContext{Title: "Gunung Lewotobi erupsi"}); len(got) != 2 {
		t.Fatalf("identical claim and title add no query of their own: %v", got)
	}
	if got := factQueries("", factContext{}); len(got) != 0 {
		t.Fatalf("nothing to search: %v", got)
	}
}
