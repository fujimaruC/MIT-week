// Command gendemo regenerates the bundled demo cases (TECH_SPEC §9).
//
// Every demo verdict is computed by the real scoring package and every ELA
// number comes from the real forensics package run on a synthetic
// screenshot, so a judge clicking a demo sees exactly what the algorithm
// does. Output is deterministic; run it from the backend directory:
//
//	go run ./cmd/gendemo
//
// It writes internal/demo/demo-cases.json (canonical), the byte-identical
// frontend copy, and the demo screenshot under frontend/static/demo/.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/amba/amba/backend/internal/forensics"
	"github.com/amba/amba/backend/internal/linkcheck"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/pipeline"
	"github.com/amba/amba/backend/internal/provenance"
	"github.com/amba/amba/backend/internal/scoring"
)

type demoCase struct {
	ID      string         `json:"id"`
	Caption string         `json:"caption"`
	Image   string         `json:"image,omitempty"`
	Dossier models.Dossier `json:"dossier"`
}

const (
	canonical    = "internal/demo/demo-cases.json"
	frontendCopy = "../frontend/src/lib/data/demo-cases.json"
	imageFile    = "../frontend/static/demo/berita-ig.jpg"
	imagePath    = "/demo/berita-ig.jpg"
	aiImageFile  = "../frontend/static/demo/gambar-ai.jpg"
	aiImagePath  = "/demo/gambar-ai.jpg"
	placeholder  = "https://example.org/contoh-cek-fakta"
)

func main() {
	img, box, err := screenshot()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(imageFile), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(imageFile, img, 0o644); err != nil {
		log.Fatal(err)
	}

	aiImg, err := aiPicture()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(aiImageFile, aiImg, 0o644); err != nil {
		log.Fatal(err)
	}

	cases := []demoCase{tiktok(), beritaIG(img, box), gambarAI(aiImg), tautan(), resmi()}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{"cases": cases}); err != nil {
		log.Fatal(err)
	}
	for _, path := range []string{canonical, frontendCopy} {
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("wrote %d cases, screenshot %d bytes\n", len(cases), len(img))

	if err := forensicQuiz(img); err != nil {
		log.Fatal(err)
	}
}

// forensicQuiz writes the PRD F7 "bedah forensik" images: three synthetic
// posts and the ELA heat map AMBA computes for each, so the Edukasi quiz can
// reveal real forensic output offline.
func forensicQuiz(edited []byte) error {
	const dir = "../frontend/static/edukasi"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// 1. An untouched post, saved once.
	var original bytes.Buffer
	if err := jpeg.Encode(&original, poster("LIBUR SEKOLAH", "MULAI SENIN", "INFO RESMI", color.RGBA{38, 70, 83, 255}, color.RGBA{220, 220, 220, 255}), &jpeg.Options{Quality: 85}); err != nil {
		return err
	}
	// 2. The edited news post from the demo case.
	// 3. A lossless screenshot: ELA lights up every edge the same way.
	var shot bytes.Buffer
	if err := png.Encode(&shot, poster("CHAT GRUP", "BESOK GRATIS", "SEBARKAN!", color.RGBA{60, 60, 70, 255}, color.RGBA{255, 214, 10, 255})); err != nil {
		return err
	}

	items := []struct {
		id    string
		data  []byte
		ext   string
		label string
	}{
		{"asli", original.Bytes(), "jpg", "asli"},
		{"diedit", edited, "jpg", "diedit"},
		{"tangkapan", shot.Bytes(), "png", "belum-pasti"},
	}
	type entry struct {
		ID     string `json:"id"`
		Image  string `json:"image"`
		ELA    string `json:"ela"`
		Answer string `json:"answer"`
	}
	var out []entry
	for _, it := range items {
		// The real pipeline must accept every quiz image.
		if _, err := forensics.Analyze(it.data); err != nil {
			return err
		}
		heat, err := heatmap(it.data)
		if err != nil {
			return err
		}
		name := "quiz-" + it.id
		if err := os.WriteFile(filepath.Join(dir, name+"."+it.ext), it.data, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name+"-ela.jpg"), heat, 0o644); err != nil {
			return err
		}
		out = append(out, entry{ID: it.id, Image: "/edukasi/" + name + "." + it.ext, ELA: "/edukasi/" + name + "-ela.jpg", Answer: it.label})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{"items": out}); err != nil {
		return err
	}
	return os.WriteFile("../frontend/src/lib/data/forensic-quiz.json", buf.Bytes(), 0o644)
}

// poster draws a simple synthetic announcement card. Saturated colours light
// up in ELA on their own, so the untouched example avoids them.
func poster(line1, line2, line3 string, bg, accent color.RGBA) *image.RGBA {
	const w, h = 360, 480
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := uint8((x*7 + y*13 + (x*y)%11) % 9)
			img.SetRGBA(x, y, color.RGBA{bg.R + n, bg.G + n, bg.B + n, 255})
		}
	}
	drawText(img, line1, 24, 120, 4, color.RGBA{241, 241, 241, 255})
	drawText(img, line2, 24, 180, 4, color.RGBA{241, 241, 241, 255})
	drawText(img, line3, 24, 260, 3, accent)
	fill(img, image.Rect(0, 412, w, 444), color.RGBA{214, 40, 40, 255})
	drawText(img, "CONTOH / SIMULASI", (w-textWidth("CONTOH / SIMULASI", 2))/2, 421, 2, color.RGBA{255, 255, 255, 255})
	return img
}

// heatmap is the ELA difference image: re-encode at quality 90, amplify the
// per-pixel difference and colour it from dark to warm.
func heatmap(encoded []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	var re bytes.Buffer
	if err := jpeg.Encode(&re, src, &jpeg.Options{Quality: forensics.JPEGQuality}); err != nil {
		return nil, err
	}
	again, err := jpeg.Decode(&re)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r1, g1, b1, _ := src.At(x, y).RGBA()
			r2, g2, b2, _ := again.At(x, y).RGBA()
			d := (absDiff(r1, r2) + absDiff(g1, g2) + absDiff(b1, b2)) / 3 >> 8
			v := min(255, int(d)*14)
			// dark -> violet -> orange -> yellow
			out.SetRGBA(x, y, color.RGBA{uint8(min(255, v*2)), uint8(max(0, v*2-255)), uint8(max(0, 120-v/2)), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// base is the shared skeleton of every demo dossier.
func base(id, ts string, input models.InputType, ref string) models.Dossier {
	return models.Dossier{
		SchemaVersion:   2,
		ID:              id,
		Timestamp:       ts,
		InputType:       input,
		SourceReference: ref,
		IsDemo:          true,
		Tactics:         []models.Tactic{},
		Pillars: models.Pillars{
			ELA:  models.ELAPillar{Anomalies: []models.Anomaly{}},
			AI:   models.AIPillar{Indicators: []string{}, Provenance: noProvenance},
			Fact: models.FactPillar{Status: models.FactUnverified, Sources: []models.Source{}},
			Link: models.LinkPillar{RedirectChain: []string{}, Reasons: []models.LinkReason{}},
		},
	}
}

// finish computes the verdict exactly as a live audit would.
func finish(d models.Dossier, res pipeline.Result, title, explanation string) models.Dossier {
	res.InputType = d.InputType
	res.Pillars = d.Pillars
	v := pipeline.Verdict(res, false)
	v.ShortTitle = title
	v.Explanation = explanation
	d.Verdict = v
	return d
}

// noProvenance is an image that declares nothing about its origin.
var noProvenance = models.Provenance{Kind: models.ProvenanceNone}

func exampleSource(title string, match models.FactStatus) []models.Source {
	return []models.Source{{Title: title, Publisher: "Sumber Contoh", URL: placeholder, Match: match}}
}

// tiktok is a TikTok link whose caption repeats a debunked aid claim: the
// link itself is the real platform, the caption is the hoax.
func tiktok() demoCase {
	link := "https://www.tiktok.com/@contoh.simulasi/video/7300000000000000001"
	caption := "Bantuan Rp 500.000 dibagikan hari ini saja. Sebarkan ke 10 grup, jangan sampai ketinggalan!"
	d := base("AMBA-20261030-DMO1", "2026-10-30T07:22:15Z", models.InputURL, link)
	analysis := linkcheck.Analyze(link)
	d.Pillars.Link = models.LinkPillar{Available: true, Score: analysis.Score, FinalURL: link,
		RedirectChain: []string{link}, Reasons: reasonsOf(analysis)}
	d.SourceMetadata = &models.SourceMetadata{Status: models.MetadataOK, Platform: "tiktok", Via: "oembed",
		Title: caption, Author: "contoh.simulasi", SiteName: "TikTok"}
	d.Pillars.Fact = models.FactPillar{Available: true, Claim: "Bantuan Rp 500.000 dibagikan hari ini",
		Status: models.FactContradicts, Sources: exampleSource("Contoh artikel pemeriksaan fakta", models.FactContradicts)}
	d.Tactics = []models.Tactic{
		{Code: "URGENCY", Phrase: "hari ini saja", Source: "text"},
		{Code: "REWARD_BAIT", Phrase: "Bantuan Rp 500.000", Source: "text"},
	}
	d = finish(d, pipeline.Result{}, "Contoh: video TikTok berisi klaim bansos",
		"Tautannya mengarah ke TikTok asli, tetapi keterangan videonya memuat klaim bantuan yang dibantah sumber contoh. Keterangannya juga memakai tekanan waktu dan meminta disebarkan. Ini data simulasi untuk memperlihatkan cara kerja AMBA.")
	return demoCase{ID: "demo-tiktok", Caption: "Video TikTok: bantuan Rp 500.000 dibagikan hari ini saja", Dossier: d}
}

func reasonsOf(analysis linkcheck.Result) []models.LinkReason {
	reasons := make([]models.LinkReason, 0, len(analysis.Findings))
	for _, f := range analysis.Findings {
		reasons = append(reasons, models.LinkReason{Code: f.Code, Text: pipeline.LinkReasonText(f.Code)})
	}
	return reasons
}

func beritaIG(img []byte, box [4]int) demoCase {
	res, err := forensics.Analyze(img)
	if err != nil {
		log.Fatal(err)
	}
	var hotspots []scoring.Box
	for _, h := range res.Hotspots(1.5) {
		hotspots = append(hotspots, scoring.Box(h))
	}
	ocr := "BERITA.CONTOH HARGA BERAS TURUN JADI RP 2.000/KG SEBARKAN SEBELUM DIHAPUS! CONTOH / SIMULASI"

	// Timpa Teks on the box the model would mark, exactly as the pipeline
	// merges it: the evidence raises the ELA score and, from 70, the box is a
	// hotspot ELA_CORROBORATED can match.
	src, _, err := forensics.Decode(img)
	if err != nil {
		log.Fatal(err)
	}
	anomaly := models.Anomaly{BoundingBox: box, ExtractedText: "RP 2.000/KG",
		Reason: "Angka harga punya tepi dan kompresi berbeda dari teks di sekitarnya"}
	score := res.Score
	if ev := forensics.TextEvidence(src, [][4]int{box})[0]; ev >= 0 {
		anomaly.Evidence = &ev
		score = max(score, ev)
		if ev >= pipeline.TextHotspotMin {
			hotspots = append(hotspots, scoring.Box(box))
		}
	}

	d := base("AMBA-20261030-DMO2", "2026-10-30T07:23:40Z", models.InputFile, "Gambar yang diunggah")
	d.Pillars.ELA = models.ELAPillar{
		Available: true, Score: score, Reliability: res.Reliability,
		Anomalies: []models.Anomaly{anomaly},
	}
	if res.Note != "" {
		note := res.Note
		d.Pillars.ELA.ReliabilityNote = &note
	}
	// A pasted price is an edit, not AI generation: the two checks answer
	// different questions.
	d.Pillars.AI = models.AIPillar{Available: true, Score: 12, Reliability: 0.5, Caveat: "Indikatif, bukan bukti pasti",
		Indicators: []string{"Tampak seperti tangkapan layar unggahan biasa"}, Provenance: noProvenance}
	d.Pillars.Fact = models.FactPillar{Available: true, Claim: "Harga beras turun menjadi Rp 2.000 per kilogram",
		Status: models.FactContradicts, Sources: exampleSource("Contoh: harga beras tidak turun ke Rp 2.000", models.FactContradicts)}
	d.Tactics = []models.Tactic{{Code: "URGENCY", Phrase: "SEBARKAN SEBELUM DIHAPUS", Source: "ocr"}}
	d.OCRText = ocr
	d = finish(d, pipeline.Result{ELAHotspots: hotspots, AIAnomalies: []scoring.Box{scoring.Box(box)}},
		"Contoh: tangkapan layar berita diedit",
		"Angka harga pada gambar ini tampak ditempel: kompresinya berbeda dari teks lain di gambar. Sumber contoh juga membantah klaimnya. Geser penanda untuk membandingkan gambar asli dan hasil ELA. Ini data simulasi.")
	return demoCase{ID: "demo-berita-ig", Caption: "Tangkapan layar berita: harga beras turun jadi Rp 2.000/kg", Image: imagePath, Dossier: d}
}

// gambarAI is an uploaded picture whose Content Credentials declare it was
// generated by AI. The bundled file really carries the marker, so the live
// app reaches the same verdict when it is uploaded.
func gambarAI(img []byte) demoCase {
	prov := provenance.Scan(img)
	if prov.Kind != provenance.AIGenerated {
		log.Fatalf("demo AI picture does not declare AI: %+v", prov)
	}
	res, err := forensics.Analyze(img)
	if err != nil {
		log.Fatal(err)
	}
	d := base("AMBA-20261030-DMO5", "2026-10-30T07:25:10Z", models.InputFile, "Gambar yang diunggah")
	d.Pillars.ELA = models.ELAPillar{Available: true, Score: res.Score, Reliability: res.Reliability, Anomalies: []models.Anomaly{}}
	if res.Note != "" {
		note := res.Note
		d.Pillars.ELA.ReliabilityNote = &note
	}
	d.Pillars.AI = models.AIPillar{Available: true, Score: 95, Reliability: 1, Caveat: "Indikatif, bukan bukti pasti",
		Indicators: []string{"Ikan dan pantai tampak seperti gambar, bukan foto"},
		Provenance: models.Provenance{Kind: models.ProvenanceAIGenerated, Source: prov.Source, Generator: prov.Generator, Camera: prov.Camera, C2PA: prov.C2PA}}
	d.Pillars.Fact = models.FactPillar{Available: true, Claim: "Ikan raksasa terdampar di pantai", Status: models.FactUnverified, Sources: []models.Source{}}
	d.OCRText = "IKAN RAKSASA TERDAMPAR! CONTOH / SIMULASI"
	d = finish(d, pipeline.Result{}, "Contoh: foto viral buatan AI",
		"Content Credentials di dalam file menyatakan gambar ini dibuat dengan AI, jadi gambar ini bukan foto kejadian nyata. Klaim di gambarnya juga belum dibahas sumber terpercaya. Ini data simulasi.")
	return demoCase{ID: "demo-gambar-ai", Caption: "Foto viral: ikan raksasa terdampar di pantai", Image: aiImagePath, Dossier: d}
}

// aiPicture draws the AI demo picture and embeds a Content Credentials
// (C2PA) segment declaring trainedAlgorithmicMedia, as AI generators write
// it. The manifest is a simulation: it is not signed.
func aiPicture() ([]byte, error) {
	const w, h = 360, 480
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := uint8((x*3 + y*5) % 7)
			switch {
			case y < 200: // sky
				img.SetRGBA(x, y, color.RGBA{uint8(120 + y/4), uint8(170 + y/8), 235, 255})
			case y < 300: // sea
				img.SetRGBA(x, y, color.RGBA{30 + n, 110 + n, 150 + n, 255})
			default: // sand
				img.SetRGBA(x, y, color.RGBA{214 + n, 190 + n, 140 + n, 255})
			}
		}
	}
	// The fish: an ellipse body and a triangular tail.
	for y := 280; y < 380; y++ {
		for x := 40; x < 330; x++ {
			dx, dy := float64(x-170)/130, float64(y-330)/45
			if dx*dx+dy*dy <= 1 {
				shade := uint8(90 + (x*7+y*3)%25)
				img.SetRGBA(x, y, color.RGBA{shade, shade + 10, shade + 25, 255})
			}
			if x >= 290 && x < 330 && abs(y-330) <= (x-290) {
				img.SetRGBA(x, y, color.RGBA{80, 90, 110, 255})
			}
		}
	}
	fill(img, image.Rect(86, 318, 94, 326), color.RGBA{20, 20, 20, 255}) // eye
	drawText(img, "IKAN RAKSASA", 24, 40, 4, color.RGBA{255, 255, 255, 255})
	drawText(img, "TERDAMPAR!", 24, 90, 4, color.RGBA{255, 214, 10, 255})
	fill(img, image.Rect(0, 412, w, 444), color.RGBA{214, 40, 40, 255})
	drawText(img, "CONTOH / SIMULASI", (w-textWidth("CONTOH / SIMULASI", 2))/2, 421, 2, color.RGBA{255, 255, 255, 255})

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	manifest := []byte("JP\x02\x11\x00\x00\x00\x01\x00\x00\x00\x40jumb\x00\x00\x00\x28jumdc2pa" +
		"c2pa.claim claim_generator AMBA-contoh-simulasi/1.0 c2pa.actions c2pa.created " +
		"digitalSourceType http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia")
	n := len(manifest) + 2
	seg := append([]byte{0xFF, 0xEB, byte(n >> 8), byte(n)}, manifest...)
	data := out.Bytes()
	withC2PA := append(append(append([]byte{}, data[:2]...), seg...), data[2:]...)
	return withC2PA, nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func tautan() demoCase {
	link := "https://bri-verifikasi.example/login"
	analysis := linkcheck.Analyze(link)
	d := base("AMBA-20261030-DMO3", "2026-10-30T07:24:00Z", models.InputURL, link)
	d.Pillars.Link = models.LinkPillar{Available: true, Score: analysis.Score, FinalURL: link,
		RedirectChain: []string{link}, Reasons: reasonsOf(analysis)}
	d.SourceMetadata = &models.SourceMetadata{Status: models.MetadataOK, Platform: "web", Via: "opengraph",
		Title: "Akun Anda akan diblokir", Description: "Verifikasi sekarang di tautan berikut.", SiteName: "Contoh Simulasi"}
	d.Tactics = []models.Tactic{
		{Code: "FEAR", Phrase: "Akun Anda akan diblokir", Source: "text"},
		{Code: "URGENCY", Phrase: "Verifikasi sekarang", Source: "text"},
	}
	d = finish(d, pipeline.Result{}, "Contoh: tautan meniru halaman masuk bank",
		"Alamat tautan memakai nama bank dengan kata umpan dan mengarah ke halaman masuk. Bukti dari tautan saja sudah cukup kuat untuk memberi peringatan. Domain .example tidak bisa didaftarkan siapa pun. Ini data simulasi.")
	return demoCase{ID: "demo-tautan", Caption: "Akun Anda akan diblokir. Verifikasi sekarang di tautan berikut.", Dossier: d}
}

// resmi is an official government link whose page states a claim a source
// supports: the one demo that ends SAFE.
func resmi() demoCase {
	link := "https://www.kemkes.go.id/id/jadwal-imunisasi"
	d := base("AMBA-20261030-DMO4", "2026-10-30T07:26:00Z", models.InputURL, link)
	analysis := linkcheck.Analyze(link)
	d.Pillars.Link = models.LinkPillar{Available: true, Score: analysis.Score, FinalURL: link,
		RedirectChain: []string{link}, Reasons: reasonsOf(analysis)}
	d.SourceMetadata = &models.SourceMetadata{Status: models.MetadataOK, Platform: "web", Via: "opengraph",
		Title: "Jadwal imunisasi anak (contoh)", Description: "Imunisasi dasar anak gratis di posyandu dan puskesmas.", SiteName: "Contoh Simulasi"}
	d.Pillars.Fact = models.FactPillar{Available: true, Claim: "Imunisasi dasar anak gratis di posyandu",
		Status: models.FactSupports, Sources: exampleSource("Contoh pengumuman resmi", models.FactSupports)}
	d = finish(d, pipeline.Result{}, "Contoh: tautan situs resmi",
		"Tautan mengarah ke situs pemerintah tanpa tanda penipuan, dan klaim di halamannya didukung sumber contoh. Tetap cek tanggal dan isi halamannya. Ini data simulasi.")
	return demoCase{ID: "demo-resmi", Caption: "Tautan situs resmi: jadwal imunisasi anak", Dossier: d}
}

// screenshot draws a synthetic news post, saves it as JPEG, then pastes an
// edited price over it the way a real hoax edit is made, and saves again.
// The pasted region has a different compression history, which is what ELA
// detects. It returns the final JPEG and the edit box (0-1000, ymin xmin
// ymax xmax).
func screenshot() ([]byte, [4]int, error) {
	const w, h = 360, 480
	navy := color.RGBA{29, 53, 87, 255}
	draw := func(price string) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		fill(img, img.Bounds(), color.RGBA{250, 250, 250, 255})
		// Header: avatar and account name.
		fill(img, image.Rect(12, 12, 40, 40), color.RGBA{200, 200, 210, 255})
		drawText(img, "berita.contoh", 50, 19, 2, color.RGBA{38, 38, 38, 255})
		// Card with a gentle deterministic texture, like a photo background.
		for y := 52; y < 412; y++ {
			for x := 0; x < w; x++ {
				n := uint8((x*7 + y*13 + (x*y)%11) % 9)
				img.SetRGBA(x, y, color.RGBA{navy.R + n, navy.G + n, navy.B + n, 255})
			}
		}
		drawText(img, "HARGA BERAS", 24, 100, 4, color.RGBA{241, 241, 241, 255})
		drawText(img, "TURUN JADI", 24, 150, 4, color.RGBA{241, 241, 241, 255})
		drawText(img, price, 24, 200, 4, color.RGBA{241, 241, 241, 255})
		drawText(img, "SEBARKAN SEBELUM DIHAPUS!", 24, 290, 2, color.RGBA{255, 214, 10, 255})
		// Watermark band (TECH_SPEC §9).
		fill(img, image.Rect(0, 372, w, 404), color.RGBA{214, 40, 40, 255})
		drawText(img, "CONTOH / SIMULASI", (w-textWidth("CONTOH / SIMULASI", 2))/2, 381, 2, color.RGBA{255, 255, 255, 255})
		drawText(img, "mulai besok harga turun!", 12, 430, 2, color.RGBA{38, 38, 38, 255})
		return img
	}

	// 1. The "original" post, as it circulated after a few re-shares: JPEG
	// quality 55, so its fine texture is already quantised away.
	var first bytes.Buffer
	if err := jpeg.Encode(&first, draw("RP 12.000/KG"), &jpeg.Options{Quality: 55}); err != nil {
		return nil, [4]int{}, err
	}
	decoded, err := jpeg.Decode(&first)
	if err != nil {
		return nil, [4]int{}, err
	}
	edited := image.NewRGBA(decoded.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			edited.Set(x, y, decoded.At(x, y))
		}
	}

	// 2. The edit: a fresh, never-compressed patch with the new price, in a
	// slightly brighter white, pasted over the old line. Its background is a
	// newly drawn texture, so it carries detail the re-shared original lost.
	patch := image.Rect(16, 192, 340, 236)
	for y := patch.Min.Y; y < patch.Max.Y; y++ {
		for x := patch.Min.X; x < patch.Max.X; x++ {
			n := uint8((x*5 + y*17 + (x*y)%13) % 13)
			edited.SetRGBA(x, y, color.RGBA{navy.R + n, navy.G + n, navy.B + n, 255})
		}
	}
	drawText(edited, "RP 2.000/KG", 24, 200, 4, color.RGBA{255, 255, 255, 255})

	// 3. Re-saved once at high quality, as an editing app would.
	var out bytes.Buffer
	if err := jpeg.Encode(&out, edited, &jpeg.Options{Quality: 96}); err != nil {
		return nil, [4]int{}, err
	}
	box := [4]int{patch.Min.Y * 1000 / h, patch.Min.X * 1000 / w, patch.Max.Y * 1000 / h, patch.Max.X * 1000 / w}
	return out.Bytes(), box, nil
}
