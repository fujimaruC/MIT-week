package pipeline

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/amba/amba/backend/internal/grounding"
	"github.com/amba/amba/backend/internal/llm"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/video"
)

// countingGateway counts model calls by kind. Classification answers follow
// the rule: a title that starts with [HOAKS] or [SALAH] is a hoax / contradicts,
// anything else is real / supports.
type countingGateway struct {
	mu     sync.Mutex
	counts map[string]int
	claim  string
}

func (g *countingGateway) serve(t *testing.T) *httptest.Server {
	t.Helper()
	g.counts = map[string]int{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		kind := "other"
		for _, k := range []string{"ClaimMatches", "WebMatches", "VideoFindings", "LLMFindings"} {
			if strings.Contains(body, k) {
				kind = k
				break
			}
		}
		g.mu.Lock()
		g.counts[kind]++
		g.mu.Unlock()
		var inner string
		switch kind {
		case "ClaimMatches", "WebMatches":
			var matches []map[string]any
			for i := 0; ; i++ {
				marker := `[` + string(rune('0'+i)) + `] Judul: `
				at := strings.Index(body, marker)
				if at < 0 {
					break
				}
				line := body[at+len(marker):]
				hoax := strings.HasPrefix(line, "[HOAKS]") || strings.HasPrefix(line, "[SALAH]")
				m := "UNVERIFIED"
				switch {
				case kind == "ClaimMatches" && hoax:
					m = "CONTRADICTS"
				case kind == "ClaimMatches":
					m = "SUPPORTS"
				case hoax:
					m = "HOAX"
				default:
					m = "REAL"
				}
				matches = append(matches, map[string]any{"index": i, "match": m, "says": m})
			}
			b, _ := json.Marshal(map[string]any{"matches": matches})
			inner = string(b)
		case "VideoFindings":
			b, _ := json.Marshal(videoFindingsMap(g.claim, "Banjir besar di Jakarta.", 10))
			inner = string(b)
		default:
			b, _ := json.Marshal(map[string]any{
				"claim": g.claim, "search_query": "", "ai_score": 10, "ai_indicators": []string{}, "ai_watermark": "",
				"text_anomalies": []any{}, "ocr_text": "", "tactics": []any{}, "injection_suspected": false,
			})
			inner = string(b)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": inner}}},
		})
	}))
}

func (g *countingGateway) total() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, v := range g.counts {
		n += v
	}
	return n
}

var (
	onTopicPages = []grounding.Result{
		{Title: "Video banjir besar hari ini ternyata buatan AI", Publisher: "kompas.com", URL: "https://www.kompas.com/cekfakta/read/1"},
		{Title: "[HOAKS] Banjir besar hari ini di Jakarta", Publisher: "turnbackhoax.id", URL: "https://turnbackhoax.id/articles/1-hoaks-banjir"},
	}
	offTopicPages = []grounding.Result{
		{Title: "[SALAH] Makanan GMO Merusak Genetik", Publisher: "turnbackhoax.id", URL: "https://turnbackhoax.id/articles/2-gmo"},
		{Title: "[SALAH] Undur-Undur Mampu Obati Diabetes", Publisher: "turnbackhoax.id", URL: "https://turnbackhoax.id/articles/3-undur"},
	}
)

// countCalls runs one audit and returns the model calls per kind.
func countCalls(t *testing.T, req Request, clip *video.Clip, web, fact []grounding.Result) (map[string]int, Result) {
	t.Helper()
	g := &countingGateway{claim: "Banjir besar menerjang Jakarta hari ini"}
	gw := g.serve(t)
	defer gw.Close()
	p, srv := pipelineWithFetch(t, false, ogPage)
	defer srv.Close()
	p.LLM = llm.New(gw.URL, "k", "m", false)
	p.Searcher = &fakeSearcher{results: fact}
	if web != nil {
		p.WebSearch = &fakeSearcher{results: web}
		p.WebGrounder = grounding.New(grounding.WebAllowlist)
	}
	if clip != nil {
		p.Video = &fakeVideo{clip: clip}
	}
	res := p.Run(context.Background(), req)
	return g.counts, res
}

func sum(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// Model calls per audit. Before this change: video 3 (video, web classify,
// claim classify) in every case that found pages; image 2.
func TestModelCallsPerAudit(t *testing.T) {
	urlReq := Request{InputType: models.InputURL, URL: tiktokURL}
	strong := testClip(t)
	strong.Title = "Banjir besar hari ini #veo3 AI generated"
	for _, tc := range []struct {
		name       string
		req        Request
		clip       *video.Clip
		web, fact  []grounding.Result
		want       int
		wantBefore int
	}{
		// 3, not 2: the claim classifier judges every page again (v2), since reusing the
		// web check's verdicts let unrelated "hoax" pages contradict a real video (10 Nov).
		{"video, no disclosure, web and fact find the same pages", urlReq, testClip(t), onTopicPages, onTopicPages, 3, 3},
		{"video, no disclosure, web and fact find different pages", urlReq, testClip(t), onTopicPages[:1],
			[]grounding.Result{{Title: "[HOAKS] Banjir besar Jakarta hari ini hoaks", Publisher: "turnbackhoax.id", URL: "https://turnbackhoax.id/articles/9-x"}}, 3, 3},
		{"video, strong creator disclosure", urlReq, strong, onTopicPages, onTopicPages, 2, 3},
		{"video, claim search returns nothing relevant", urlReq, testClip(t), onTopicPages[:1], offTopicPages, 2, 3},
		{"video, no search results at all", urlReq, testClip(t), nil, nil, 1, 1},
		{"image, relevant fact-check pages", imageRequest(t, 3, ""), nil, nil, onTopicPages, 2, 2},
		{"image, only unrelated pages", imageRequest(t, 3, ""), nil, nil, offTopicPages, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counts, res := countCalls(t, tc.req, tc.clip, tc.web, tc.fact)
			t.Logf("calls=%d (before this change: %d) %v", sum(counts), tc.wantBefore, counts)
			if tc.want >= 0 && sum(counts) != tc.want {
				t.Fatalf("calls = %d, want %d: %v", sum(counts), tc.want, counts)
			}
			if res.Pillars.Fact.Available && len(res.Pillars.Fact.Sources) > 5 {
				t.Fatalf("too many sources")
			}
		})
	}
}

func TestFactSourcesAreOnlyTheRelevantOnesAndAtMostFive(t *testing.T) {
	var many []grounding.Result
	for i := 0; i < 8; i++ {
		many = append(many, grounding.Result{
			Title: "Banjir besar menerjang Jakarta hari ini versi " + string(rune('a'+i)), Publisher: "kompas.com",
			URL: "https://www.kompas.com/cekfakta/read/" + string(rune('1'+i)),
		})
	}
	mixed := append(append([]grounding.Result{}, offTopicPages...), many...)
	_, res := countCalls(t, imageRequest(t, 4, ""), nil, nil, mixed)
	f := res.Pillars.Fact
	if len(f.Sources) != 5 {
		t.Fatalf("sources = %d, want 5: %+v", len(f.Sources), f.Sources)
	}
	for _, s := range f.Sources {
		if strings.Contains(s.Title, "GMO") || strings.Contains(s.Title, "Undur") {
			t.Fatalf("unrelated source listed: %+v", s)
		}
	}
}

func TestNoRelevantSourceLeavesTheFactPillarUnverifiedAndEmpty(t *testing.T) {
	_, res := countCalls(t, imageRequest(t, 4, ""), nil, nil, offTopicPages)
	f := res.Pillars.Fact
	if !f.Available || f.Status != models.FactUnverified || len(f.Sources) != 0 {
		t.Fatalf("fact = %+v", f)
	}
}

func TestContradictingSourceSurvivesTheCap(t *testing.T) {
	var many []grounding.Result
	for i := 0; i < 7; i++ {
		many = append(many, grounding.Result{
			Title: "Banjir besar menerjang Jakarta hari ini versi " + string(rune('a'+i)), Publisher: "kompas.com",
			URL: "https://www.kompas.com/news/" + string(rune('1'+i)),
		})
	}
	many = append(many, grounding.Result{Title: "[HOAKS] Banjir besar menerjang Jakarta hari ini", Publisher: "turnbackhoax.id", URL: "https://turnbackhoax.id/articles/5-hoaks"})
	_, res := countCalls(t, imageRequest(t, 4, ""), nil, nil, many)
	f := res.Pillars.Fact
	if f.Status != models.FactContradicts || len(f.Sources) != 5 || f.Sources[0].Match != models.FactContradicts {
		t.Fatalf("fact = %+v", f)
	}
}
