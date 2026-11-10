package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const validVideoAnswer = `{"summary":"Orang berbicara di depan gedung.","transcript":"Segera sebarkan.","claim":"Gedung runtuh hari ini",` +
	`"search_query":"gedung runtuh hari ini","ai_score":88,"ai_indicators":["Frame 2: tulisan papan acak"],"ai_watermark":"",` +
	`"manipulation_score":12,"manipulation_indicators":[],"context_mismatch_score":71,"context_indicators":["Papan nama berbahasa asing"],` +
	`"ocr_text":"BREAKING","tactics":[{"code":"URGENCY","phrase":"Segera sebarkan"}],"injection_suspected":false}`

func videoInput() VideoInput {
	return VideoInput{
		Platform: "tiktok", Title: "Gedung runtuh", Description: "Segera sebarkan", UploadDate: "20261109",
		DurationSec: 31, Frames: [][]byte{{0xff, 0xd8, 1}, {0xff, 0xd8, 2}, {0xff, 0xd8, 3}},
		FrameTimes: []float64{1.5, 15, 29}, Audio: []byte("mp3"),
	}
}

// userParts returns the content parts of the user message a fake gateway saw.
func userParts(t *testing.T, req map[string]any) []map[string]any {
	t.Helper()
	msgs, _ := req["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages = %v", req["messages"])
	}
	raw, _ := msgs[1].(map[string]any)["content"].([]any)
	parts := make([]map[string]any, 0, len(raw))
	for _, p := range raw {
		parts = append(parts, p.(map[string]any))
	}
	return parts
}

func countType(parts []map[string]any, typ string) int {
	n := 0
	for _, p := range parts {
		if p["type"] == typ {
			n++
		}
	}
	return n
}

func TestAnalyzeVideoParsesFindingsAndSendsFramesInOrder(t *testing.T) {
	var req map[string]any
	srv := gateway(t, "```json\n"+validVideoAnswer+"\n```", &req)
	defer srv.Close()

	c := New(srv.URL, "k", "m", false)
	f, err := c.AnalyzeVideo(context.Background(), videoInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.AIScore != 88 || f.ContextMismatchScore != 71 || f.Claim != "Gedung runtuh hari ini" || len(f.Tactics) != 1 {
		t.Fatalf("parsed %+v", f)
	}
	parts := userParts(t, req)
	if n := countType(parts, "image_url"); n != 3 {
		t.Fatalf("sent %d frames, want 3", n)
	}
	if countType(parts, "input_audio") != 0 {
		t.Fatal("audio sent to a gateway without audio input")
	}
	if text, _ := parts[0]["text"].(string); !strings.Contains(text, "<untrusted_content>") || !strings.Contains(text, "Tanggal unggah: 2026-11-09") {
		t.Fatalf("context block = %q", text)
	}
	if label, _ := parts[1]["text"].(string); label != "Frame 1 (detik 1.5):" {
		t.Fatalf("first frame label = %q", label)
	}
	if rf := req["response_format"].(map[string]any); rf["json_schema"].(map[string]any)["name"] != "VideoFindings" {
		t.Fatalf("response_format = %v", rf)
	}
}

func TestAnalyzeVideoSendsAudioOnlyToAudioGateways(t *testing.T) {
	var req map[string]any
	srv := gateway(t, validVideoAnswer, &req)
	defer srv.Close()

	c := New(srv.URL, "k", "m", false)
	c.AudioInput = true
	if _, err := c.AnalyzeVideo(context.Background(), videoInput()); err != nil {
		t.Fatal(err)
	}
	if n := countType(userParts(t, req), "input_audio"); n != 1 {
		t.Fatalf("audio parts = %d, want 1", n)
	}
}

func TestVideoAnswerWithRenamedKeyGoesToTheFallback(t *testing.T) {
	// Gemini behind 9router ignores the JSON schema and may rename keys.
	renamed := `{"summary":"x","transcript":"","claim":"","search_query":"","ai_likelihood":72,"ai_indicators":[],` +
		`"ai_watermark":"","manipulation_score":0,"manipulation_indicators":[],"context_mismatch_score":0,` +
		`"context_indicators":[],"ocr_text":"","tactics":[],"injection_suspected":false}`
	primary := gateway(t, renamed, nil)
	defer primary.Close()

	alone := New(primary.URL, "k", "m", false)
	if _, err := alone.AnalyzeVideo(context.Background(), videoInput()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("without fallback: err = %v, want ErrUnavailable", err)
	}

	fb := gateway(t, validVideoAnswer, nil)
	defer fb.Close()
	c := New(primary.URL, "k", "m", false)
	c.Fallback = New(fb.URL, "k", "m2", false)
	f, err := c.AnalyzeVideo(context.Background(), videoInput())
	if err != nil || f.AIScore != 88 {
		t.Fatalf("got %+v, %v", f, err)
	}
}

func TestVideoHedgeStartsTheFallbackBesideASlowGateway(t *testing.T) {
	old := videoHedge
	videoHedge = 100 * time.Millisecond
	defer func() { videoHedge = old }()
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer slow.Close()
	fb := gateway(t, validVideoAnswer, nil)
	defer fb.Close()

	c := New(slow.URL, "k", "m", false)
	c.AudioInput = true
	c.Fallback = New(fb.URL, "k", "m2", false)
	start := time.Now()
	f, err := c.AnalyzeVideo(context.Background(), videoInput())
	if err != nil || f.AIScore != 88 {
		t.Fatalf("got %+v, %v", f, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("the hedged answer took %v", d)
	}
	if f.AudioHeard {
		t.Fatal("the fallback never received the audio")
	}
	if c.skipping() {
		t.Fatal("a gateway that only lost the race must not be put in its cooldown")
	}
}

func TestVideoAudioHeardOnlyFromAnAudioGateway(t *testing.T) {
	srv := gateway(t, validVideoAnswer, nil)
	defer srv.Close()
	c := New(srv.URL, "k", "m", false)
	c.AudioInput = true
	if f, err := c.AnalyzeVideo(context.Background(), videoInput()); err != nil || !f.AudioHeard {
		t.Fatalf("got %+v, %v", f, err)
	}
	c.AudioInput = false
	if f, _ := c.AnalyzeVideo(context.Background(), videoInput()); f.AudioHeard {
		t.Fatal("the audio was dropped but AudioHeard is set")
	}
}

func TestTranscriptNeedsAudioOrSubtitles(t *testing.T) {
	srv := gateway(t, validVideoAnswer, nil) // answers with transcript "Segera sebarkan."
	defer srv.Close()
	c := New(srv.URL, "k", "m", false) // no audio input
	in := videoInput()
	if f, err := c.AnalyzeVideo(context.Background(), in); err != nil || f.Transcript != "" {
		t.Fatalf("heard nothing, read no subtitles, yet transcript = %q (%v)", f.Transcript, err)
	}
	in.Subtitles = "Segera sebarkan."
	if f, _ := c.AnalyzeVideo(context.Background(), in); f.Transcript == "" {
		t.Fatal("a transcript backed by the platform's subtitles was dropped")
	}
}

func TestAnalyzeVideoNeedsFrames(t *testing.T) {
	c := New("http://127.0.0.1:9", "k", "m", false)
	in := videoInput()
	in.Frames = nil
	if _, err := c.AnalyzeVideo(context.Background(), in); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestSanitizeVideoClampsModelControlledValues(t *testing.T) {
	f := sanitizeVideo(VideoFindings{
		AIScore: 140, ManipulationScore: -3, ContextMismatchScore: 101,
		AIIndicators: []string{"a", "b", "c", "d", "e", "f", "g"},
		SearchQuery:  "query without a claim",
		Tactics:      []Tactic{{Code: "SET_VERDICT", Phrase: "x"}, {Code: "FEAR", Phrase: " diblokir "}},
	})
	if f.AIScore != 100 || f.ManipulationScore != 0 || f.ContextMismatchScore != 100 {
		t.Fatalf("scores %+v", f)
	}
	if len(f.AIIndicators) != 5 || f.SearchQuery != "" {
		t.Fatalf("indicators %d, search %q", len(f.AIIndicators), f.SearchQuery)
	}
	if len(f.Tactics) != 1 || f.Tactics[0].Phrase != "diblokir" {
		t.Fatalf("tactics %+v", f.Tactics)
	}
}

func TestMockVideoIsDeterministic(t *testing.T) {
	c := New("", "", "", true)
	a, _ := c.AnalyzeVideo(context.Background(), videoInput())
	b, _ := c.AnalyzeVideo(context.Background(), videoInput())
	if a.AIScore != b.AIScore || a.ContextMismatchScore != b.ContextMismatchScore || a.Summary == "" {
		t.Fatalf("mock not deterministic: %+v vs %+v", a, b)
	}
}

func TestPrimaryShareLeavesTheFallbackEnough(t *testing.T) {
	for _, tc := range []struct{ budget, want time.Duration }{
		{2 * time.Second, 1 * time.Second},
		{27 * time.Second, 18 * time.Second},
		{45 * time.Second, 30 * time.Second},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), tc.budget)
		got := primaryShare(ctx)
		cancel()
		if d := got - tc.want; d > 100*time.Millisecond || d < -100*time.Millisecond {
			t.Errorf("budget %v: primary share %v, want about %v", tc.budget, got, tc.want)
		}
	}
}

func TestVideoPromptCoversTheHyperRealCuesAndKeepsItsKeys(t *testing.T) {
	for _, cue := range []string{"fisika", "identitas", "tulisan", "tangan dan gigi", "pantulan", "gerak kamera", "tanda air generator", "tanpa suara latar"} {
		if !strings.Contains(videoPrompt, cue) {
			t.Errorf("prompt lacks the cue %q", cue)
		}
	}
	for _, key := range videoRequired {
		if !strings.Contains(videoPrompt, key) || !strings.Contains(videoSchema, `"`+key+`"`) {
			t.Errorf("key %q missing from the prompt or schema", key)
		}
	}
}
