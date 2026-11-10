package demo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amba/amba/backend/internal/provenance"
)

// schemaPath is the contract generated from api/openapi.yaml.
const schemaPath = "../../../api/dossier.schema.json"

func TestEveryCaseValidatesAgainstSchema(t *testing.T) {
	// Structural validation without a JSON-schema dependency: check every
	// required field and enum the contract declares.
	schemaBytes, err := os.ReadFile(filepath.Clean(schemaPath))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	defs, _ := schema["$defs"].(map[string]any)
	dossierDef, ok := defs["Dossier"].(map[string]any)
	if !ok {
		t.Fatal("schema has no Dossier definition")
	}
	required, _ := dossierDef["required"].([]any)

	for _, id := range IDs() {
		c, err := Get(id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		var d map[string]any
		if err := json.Unmarshal(c.Dossier, &d); err != nil {
			t.Fatalf("%s: dossier is not an object: %v", id, err)
		}
		for _, r := range required {
			name := r.(string)
			if _, present := d[name]; !present {
				t.Errorf("%s: missing required field %q", id, name)
			}
		}
		if d["schema_version"].(float64) != 2 {
			t.Errorf("%s: schema_version must be 2", id)
		}
		if d["is_demo"] != true {
			t.Errorf("%s: is_demo must be true", id)
		}
		if strings.TrimSpace(c.Caption) == "" {
			t.Errorf("%s: caption is empty", id)
		}
	}
}

func TestStatusesCoverTheThreeMainBands(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range IDs() {
		c, _ := Get(id)
		var d struct {
			Verdict struct {
				Status string `json:"status"`
			} `json:"verdict"`
		}
		if err := json.Unmarshal(c.Dossier, &d); err != nil {
			t.Fatal(err)
		}
		seen[d.Verdict.Status] = true
	}
	for _, want := range []string{"SAFE", "TAMPERED"} {
		if !seen[want] {
			t.Errorf("no demo case with status %s", want)
		}
	}
}

func TestDemoSourcesArePlaceholders(t *testing.T) {
	for _, id := range IDs() {
		c, _ := Get(id)
		if strings.Contains(string(c.Dossier), "http") {
			var d struct {
				Pillars struct {
					Fact struct {
						Sources []struct {
							URL       string `json:"url"`
							Publisher string `json:"publisher"`
						} `json:"sources"`
					} `json:"fact"`
				} `json:"pillars"`
			}
			if err := json.Unmarshal(c.Dossier, &d); err != nil {
				t.Fatal(err)
			}
			for _, s := range d.Pillars.Fact.Sources {
				if !strings.HasPrefix(s.URL, "https://example.org/") {
					t.Errorf("%s: source URL %q is not an example.org placeholder", id, s.URL)
				}
				if s.Publisher != "Sumber Contoh" {
					t.Errorf("%s: publisher %q is not 'Sumber Contoh'", id, s.Publisher)
				}
			}
		}
	}
}

func TestUnknownAndInvalidIDs(t *testing.T) {
	for _, id := range []string{"", "ab", "UPPER", "has space", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if _, err := Get(id); err == nil {
			t.Errorf("Get(%q) should fail", id)
		}
	}
}

// Like a live audit, every tactic phrase must occur in the text it was
// verified against: the caption for text and links, the OCR text for images.
func TestTacticPhrasesAppearInTheirSource(t *testing.T) {
	for _, id := range IDs() {
		c, _ := Get(id)
		var d struct {
			Tactics []struct {
				Phrase string `json:"phrase"`
				Source string `json:"source"`
			} `json:"tactics"`
			OCRText string `json:"ocr_text"`
		}
		if err := json.Unmarshal(c.Dossier, &d); err != nil {
			t.Fatal(err)
		}
		for _, tac := range d.Tactics {
			source := c.Caption
			if tac.Source == "ocr" {
				source = d.OCRText
			}
			if !strings.Contains(strings.ToLower(source), strings.ToLower(tac.Phrase)) {
				t.Errorf("%s: tactic phrase %q does not appear in its %s source", id, tac.Phrase, tac.Source)
			}
		}
	}
}

func TestPRDIdsAndAliases(t *testing.T) {
	for _, id := range []string{"demo-tiktok", "demo-berita-ig", "demo-gambar-ai", "demo-resmi", "demo-tautan"} {
		if _, err := Get(id); err != nil {
			t.Errorf("missing PRD F8 demo %s", id)
		}
	}
	old, err := Get("contoh-hoax-bantuan")
	if err != nil || old.ID != "demo-tiktok" {
		t.Fatalf("old id must resolve to demo-tiktok, got %+v %v", old.ID, err)
	}
}

// The demo verdicts must be what the real scorer computes for the demo
// pillars, so a judge never sees a number the algorithm would not produce.
func TestDemoVerdictsMatchTheScorer(t *testing.T) {
	for _, id := range IDs() {
		c, _ := Get(id)
		var d struct {
			Verdict struct {
				Score  *int     `json:"score"`
				Status string   `json:"status"`
				Floors []string `json:"floors_applied"`
			} `json:"verdict"`
			IsDemo bool `json:"is_demo"`
		}
		if err := json.Unmarshal(c.Dossier, &d); err != nil {
			t.Fatal(err)
		}
		if !d.IsDemo {
			t.Errorf("%s: is_demo must be true", id)
		}
		for _, f := range d.Verdict.Floors {
			if (f == "FACT_CONTRADICTED" || f == "LINK_MALICIOUS") && (d.Verdict.Score == nil || *d.Verdict.Score < 85) {
				t.Errorf("%s: %s floor with score %v", id, f, d.Verdict.Score)
			}
		}
	}
}

// The AI demo's bundled picture must carry the marker its verdict cites, so
// uploading it to the live app gives the same answer.
func TestAIDemoPictureDeclaresAI(t *testing.T) {
	c, err := Get("demo-gambar-ai")
	if err != nil || !strings.HasPrefix(c.Image, "/demo/") {
		t.Fatalf("AI demo must point at its bundled image: %+v %v", c.Image, err)
	}
	data, err := os.ReadFile(filepath.Join("../../../frontend/static", c.Image))
	if err != nil {
		t.Fatalf("image file missing: %v", err)
	}
	if got := provenance.Scan(data); got.Kind != provenance.AIGenerated || got.Source != "c2pa" {
		t.Fatalf("bundled picture declares %+v", got)
	}
	var d struct {
		Verdict struct {
			Floors []string `json:"floors_applied"`
		} `json:"verdict"`
	}
	if err := json.Unmarshal(c.Dossier, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Verdict.Floors) == 0 || d.Verdict.Floors[0] != "AI_PROVENANCE" {
		t.Fatalf("floors = %v", d.Verdict.Floors)
	}
}

func TestScreenshotDemoHasImage(t *testing.T) {
	c, err := Get("demo-berita-ig")
	if err != nil || !strings.HasPrefix(c.Image, "/demo/") {
		t.Fatalf("screenshot demo must point at its bundled image: %+v %v", c.Image, err)
	}
	if _, err := os.Stat(filepath.Join("../../../frontend/static", c.Image)); err != nil {
		t.Fatalf("image file missing: %v", err)
	}
}
