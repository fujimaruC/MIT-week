// Command evalimages runs the real audit pipeline over folders of labelled
// images, so the team can calibrate Jejak AI and Timpa Teks with the model
// and keys they deploy with (the thresholds were set without a model key).
//
// It reads the same environment as the server (NINEROUTER_*, LLM_MOCK,
// SIGHTENGINE_*, GOOGLE_API_KEY, ...). Each argument is a folder whose
// images share one label, the folder name, for example:
//
//	evalimages ./eval/ai ./eval/asli ./eval/teks-ditempel
//
// It prints one line per image (AI score and what decided it, provenance,
// the largest pasted-text evidence, verdict and floors) and a summary per
// folder. EVAL_DELAY_MS (default 3500, inside a free model's 20 requests a
// minute) spaces the images; EVAL_RETRIES (default 2) retries an image whose
// run degraded, e.g. on a rate limit. Nothing is uploaded anywhere except to
// the services the server itself would call; nothing is written to disk.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amba/amba/backend/internal/config"
	"github.com/amba/amba/backend/internal/httpapi"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/pipeline"
	"github.com/amba/amba/backend/internal/scoring"
	"github.com/amba/amba/backend/internal/security"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: evalimages <folder> [<folder> ...]")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration:", err)
		os.Exit(1)
	}
	deps := httpapi.NewDeps(cfg)
	if cfg.LLMMock {
		fmt.Fprintln(os.Stderr, "note: LLM_MOCK=true, the model's answers are invented; set LLM_MOCK=false to calibrate")
	}

	delay := time.Duration(envInt("EVAL_DELAY_MS", 3500)) * time.Millisecond
	retries := envInt("EVAL_RETRIES", 2)
	fmt.Println("label\tfile\tai\tai_rel\tprovenance\tdetector\ttext_evidence\tstatus\tscore\tfloors\tdegraded\tseconds")
	for _, dir := range os.Args[1:] {
		label := filepath.Base(filepath.Clean(dir))
		files, _ := filepath.Glob(filepath.Join(dir, "*"))
		sort.Strings(files)
		var n, likely, declared, pasted, available, degraded int
		var scores []int
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			img, format, _, err := security.ValidateUpload(bytes.NewReader(data), cfg.MaxUploadMB<<20)
			if err != nil {
				fmt.Printf("%s\t%s\tskipped: %v\n", label, filepath.Base(f), err)
				continue
			}
			req := pipeline.Request{InputType: models.InputFile, Image: img, ImageFormat: format}
			var res pipeline.Result
			var took time.Duration
			for attempt := 0; attempt <= retries; attempt++ {
				if attempt > 0 {
					time.Sleep(time.Duration(attempt) * 10 * time.Second)
				}
				start := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), cfg.HardTimeout)
				res = deps.Pipeline.Run(ctx, req)
				cancel()
				took = time.Since(start)
				if !res.Degraded {
					break
				}
			}
			if res.Degraded {
				degraded++
			}
			d := httpapi.BuildDossier(deps.Key, res, req)
			v := d.Verdict

			ai := d.Pillars.AI
			prov := string(ai.Provenance.Kind)
			if ai.Provenance.Source != "" {
				prov += "/" + ai.Provenance.Source
			}
			det := "-"
			if ai.Detector != nil {
				det = fmt.Sprint(ai.Detector.Score)
			}
			text := -1
			for _, a := range d.Pillars.ELA.Anomalies {
				if a.Evidence != nil && *a.Evidence > text {
					text = *a.Evidence
				}
			}
			score := "-"
			if v.Score != nil {
				score = fmt.Sprint(*v.Score)
			}
			aiScore := "-"
			n++
			if ai.Available {
				available++
				aiScore = fmt.Sprint(ai.Score)
				scores = append(scores, ai.Score)
			}
			if slices.Contains(v.FloorsApplied, scoring.FloorAILikely) {
				likely++
			}
			if slices.Contains(v.FloorsApplied, scoring.FloorAIProvenance) || slices.Contains(v.FloorsApplied, scoring.FloorAIEdited) {
				declared++
			}
			if text >= pipeline.TextHotspotMin {
				pasted++
			}
			fmt.Printf("%s\t%s\t%s\t%.1f\t%s\t%s\t%d\t%s\t%s\t%s\t%v\t%.1f\n", label, filepath.Base(f), aiScore, ai.Reliability,
				prov, det, text, v.Status, score, strings.Join(v.FloorsApplied, ","), res.Degraded, took.Seconds())
			time.Sleep(delay)
		}
		fmt.Fprintf(os.Stderr, "%s [%s]: %d images, AI judged %d, median AI score %d, AI_LIKELY %d, declared AI %d, pasted text (>= %d) %d, degraded %d\n",
			label, deps.Pipeline.LLM.Describe(), n, available, median(scores), likely, declared, pipeline.TextHotspotMin, pasted, degraded)
	}
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v >= 0 {
		return v
	}
	return def
}

func median(v []int) int {
	if len(v) == 0 {
		return -1
	}
	s := slices.Clone(v)
	slices.Sort(s)
	return s[len(s)/2]
}
