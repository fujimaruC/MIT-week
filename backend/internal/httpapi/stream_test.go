package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/amba/amba/backend/internal/config"
	"github.com/amba/amba/backend/internal/models"
)

type sseEvent struct {
	name string
	data string
}

func parseSSE(body string) []sseEvent {
	var out []sseEvent
	for _, block := range strings.Split(body, "\n\n") {
		var e sseEvent
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				e.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				e.data = strings.TrimPrefix(line, "data: ")
			}
		}
		if e.name != "" {
			out = append(out, e)
		}
	}
	return out
}

// Every pillar starts, ends exactly once, and its partial follows its own
// "done" stage; the verdict comes after every pillar settled.
func TestSSEIsProgressivePerPillar(t *testing.T) {
	rec := postJSON(t, "/api/v1/audit/url", `{"url":"https://bri-verifikasi.xyz/login"}`, "text/event-stream")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	events := parseSSE(rec.Body.String())
	ended := map[string]string{}
	lastStageDone := map[string]int{}
	verdictAt := -1
	for i, e := range events {
		switch e.name {
		case "stage":
			var s struct{ Pillar, Status string }
			_ = json.Unmarshal([]byte(e.data), &s)
			if s.Status != "running" {
				if _, dup := ended[s.Pillar]; dup {
					t.Fatalf("%s ended twice", s.Pillar)
				}
				ended[s.Pillar] = s.Status
				lastStageDone[s.Pillar] = i
			}
		case "partial":
			var p struct{ Pillar string }
			_ = json.Unmarshal([]byte(e.data), &p)
			if at, ok := lastStageDone[p.Pillar]; !ok || at > i || ended[p.Pillar] != "done" {
				t.Fatalf("partial for %s without a preceding done stage", p.Pillar)
			}
		case "verdict":
			verdictAt = i
		}
	}
	for _, p := range []string{"ela", "ai", "fact", "link"} {
		if _, ok := ended[p]; !ok {
			t.Fatalf("pillar %s never settled: %v", p, ended)
		}
		if lastStageDone[p] > verdictAt {
			t.Fatalf("pillar %s settled after the verdict", p)
		}
	}
	if ended["link"] != "done" || ended["ela"] != "skipped" {
		t.Fatalf("stages = %v", ended)
	}
	if events[len(events)-1].name != "done" {
		t.Fatalf("last event = %s", events[len(events)-1].name)
	}
}

func TestURLDossierCarriesSourceMetadata(t *testing.T) {
	// The sandboxed fetch cannot reach the host, which is exactly the
	// "could not read" case the UI has to explain.
	rec := postJSON(t, "/api/v1/audit/url", `{"url":"https://amba-test.invalid/post"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var d models.Dossier
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.SourceMetadata == nil || d.SourceMetadata.Status != models.MetadataUnavailable {
		t.Fatalf("source_metadata = %+v", d.SourceMetadata)
	}
	if !strings.Contains(d.Verdict.Explanation, "tidak menampilkan isi") {
		t.Fatalf("explanation should mention the unreadable page: %q", d.Verdict.Explanation)
	}

	img := postImage(t, testRouter(t), "")
	if !strings.Contains(img.Body.String(), `"source_metadata":null`) {
		t.Fatal("image audits must carry source_metadata: null")
	}
}

func TestExplanationUsesPlainNames(t *testing.T) {
	rec := postJSON(t, "/api/v1/audit/url", `{"url":"https://example.org/x"}`, "")
	var d models.Dossier
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	if strings.Contains(d.Verdict.Explanation, " ela") || !strings.Contains(d.Verdict.Explanation, "keamanan tautan") ||
		strings.Contains(strings.ToLower(d.Verdict.Explanation), "tidak dapat diperiksa") || strings.Contains(d.Verdict.Explanation, "teks ditimpa") {
		t.Fatalf("explanation should name pillars in plain words: %q", d.Verdict.Explanation)
	}
}

func TestInvalidURLSaysWhatIsWrong(t *testing.T) {
	rec := postJSON(t, "/api/v1/audit/url", `{"url":""}`, "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "message") {
		t.Fatalf("status %d body = %s", rec.Code, rec.Body.String())
	}
}

// The metrics endpoint counts which rules fired, by code only.
func TestRuleCountersUseCodesOnly(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	deps := NewDeps(cfg)
	router := NewRouterWith(cfg, deps, nil)
	if rec := postJSONTo(t, router, "/api/v1/audit/url", `{"url":"https://brimo-verifikasi.my.id/login"}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out strings.Builder
	deps.Metrics.Render(&out)
	body := out.String()
	for _, want := range []string{
		`amba_floors_total{floor="LINK_MALICIOUS"} 1`,
		`amba_link_reasons_total{reason="LOOKALIKE_BRAND"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in\n%s", want, body)
		}
	}
	if strings.Contains(body, "brimo-verifikasi") {
		t.Fatal("user content reached the metrics")
	}
}
