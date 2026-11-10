package forensics

import (
	"bytes"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"
)

// TestTextBenchmark reruns the Timpa Teks measurement on sets made by
// tools/textbench/gen.py. It is skipped unless TEXTBENCH lists the set
// directories (separated like PATH).
func TestTextBenchmark(t *testing.T) {
	root := os.Getenv("TEXTBENCH")
	if root == "" {
		t.Skip("set TEXTBENCH to the directories made by tools/textbench/gen.py")
	}
	type pair struct {
		ID     string            `json:"id"`
		Q      []any             `json:"q"`
		Boxes  map[string][4]int `json:"boxes"`
		Newbox [4]int            `json:"newbox"`
	}
	for _, dir := range filepath.SplitList(root) {
		raw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
		if err != nil {
			t.Fatal(err)
		}
		var pairs []pair
		if err := json.Unmarshal(raw, &pairs); err != nil {
			t.Fatal(err)
		}
		count := map[string]int{}
		for _, p := range pairs {
			for _, kind := range []string{"legit", "hoax"} {
				data, err := os.ReadFile(filepath.Join(dir, p.ID+"_"+kind+".jpg"))
				if err != nil {
					t.Fatal(err)
				}
				src, _, err := image.Decode(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				w, h := src.Bounds().Dx(), src.Bounds().Dy()
				b := p.Boxes["price"]
				if kind == "hoax" {
					b = p.Newbox
				}
				box := [4]int{b[1] * 1000 / h, b[0] * 1000 / w, b[3] * 1000 / h, b[2] * 1000 / w}
				ev := TextEvidence(src, [][4]int{box})[0]
				key := kind
				if kind == "hoax" && p.Q[3] != nil {
					key = "hoax_forwarded"
				}
				count[key]++
				switch {
				case ev < 0:
					count[key+"_unmeasured"]++
				case ev >= 70:
					count[key+"_70"]++
					count[key+"_50"]++
				case ev >= 50:
					count[key+"_50"]++
				}
			}
		}
		t.Logf("%s: original lines >=50 %d/%d; retyped >=50 %d/%d (>=70 %d), forwarded again >=50 %d/%d (>=70 %d); unmeasured %d",
			filepath.Base(dir), count["legit_50"], count["legit"],
			count["hoax_50"], count["hoax"], count["hoax_70"],
			count["hoax_forwarded_50"], count["hoax_forwarded"], count["hoax_forwarded_70"],
			count["legit_unmeasured"]+count["hoax_unmeasured"]+count["hoax_forwarded_unmeasured"])
		if count["legit_70"] > 0 {
			t.Errorf("%s: %d original lines reached the hotspot threshold", dir, count["legit_70"])
		}
	}
}
