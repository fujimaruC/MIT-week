package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// The 5W1H is optional: an answer without it, or with it in the wrong
// shape, still decodes, so a gateway that ignores the schema never fails a video.
func TestW5H1IsOptionalAndTolerant(t *testing.T) {
	base := `"summary":"s","transcript":"","claim":"","search_query":"","ai_score":10,"ai_indicators":[],"ai_watermark":"",` +
		`"manipulation_score":0,"manipulation_indicators":[],"context_mismatch_score":0,"context_indicators":[],"ocr_text":"","tactics":[],"injection_suspected":false`
	for name, body := range map[string]string{
		"missing":      "{" + base + "}",
		"a string":     `{"w5h1":"apa siapa kapan",` + base + "}",
		"a number":     `{"w5h1":5,` + base + "}",
		"wrong fields": `{"w5h1":{"what":7,"who":["x"]},` + base + "}",
	} {
		var f VideoFindings
		if err := decodeRequired(body, &f, videoRequired...); err != nil {
			t.Fatalf("%s: the answer must still decode, got %v", name, err)
		}
		if !f.W5H1.Empty() {
			t.Fatalf("%s: unreadable 5W1H is dropped, got %+v", name, f.W5H1)
		}
	}
	var f VideoFindings
	body := `{"w5h1":{"what":"Banjir menyeret mobil.","who":"Tidak diketahui.","when":"-","where":"  Jakarta  ","why":"","how":"` + strings.Repeat("a", 300) + `"},` + base + "}"
	if err := decodeRequired(body, &f, videoRequired...); err != nil {
		t.Fatal(err)
	}
	f = sanitizeVideo(f)
	w := f.W5H1
	if w.What != "Banjir menyeret mobil." || w.Who != "" || w.When != "" || w.Where != "Jakarta" {
		t.Fatalf("fillers become empty, text is trimmed: %+v", w)
	}
	if n := len([]rune(w.How)); n != 160 {
		t.Fatalf("each answer is capped at 160 runes, got %d", n)
	}
}

// Strict structured output needs every nested key required and no others.
func TestVideoSchemaAsksForTheSixQuestionsStrictly(t *testing.T) {
	var s struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			AdditionalProperties *bool    `json:"additionalProperties"`
			Required             []string `json:"required"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(videoSchema), &s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(s.Required, ","), "w5h1") {
		t.Fatal("w5h1 must be in the schema's required list for strict mode")
	}
	w, ok := s.Properties["w5h1"]
	if !ok || w.AdditionalProperties == nil || *w.AdditionalProperties || strings.Join(w.Required, ",") != "what,who,when,where,why,how" {
		t.Fatalf("w5h1 must be a closed object with six required keys: %+v", w)
	}
	for _, k := range []string{"w5h1", "what", "who", "when", "where", "why", "how"} {
		if !strings.Contains(videoPrompt, k) {
			t.Fatalf("the prompt must name %q", k)
		}
	}
	for _, k := range videoRequired {
		if k == "w5h1" {
			t.Fatal("w5h1 must stay optional for the answer check (videoRequired)")
		}
	}
}
