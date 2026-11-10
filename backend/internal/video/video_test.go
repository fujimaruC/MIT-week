package video

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess implements fake yt-dlp and ffmpeg binaries for offline tests.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	mode := os.Getenv("HELPER_MODE")

	// Find the separator "--" which marks the start of our simulated tool arguments
	args := os.Args
	sepIdx := -1
	for i, a := range args {
		if a == "--simulated-args" {
			sepIdx = i
			break
		}
	}
	if sepIdx != -1 && sepIdx+1 < len(args) {
		args = args[sepIdx+1:]
	}

	if strings.HasPrefix(mode, "ytdlp_") && strings.Contains(strings.Join(args, " "), "--dump-json") {
		if mode == "ytdlp_probe_fail" {
			fmt.Fprintln(os.Stderr, "ERROR: unable to extract")
			os.Exit(1)
		}
		if !strings.Contains(strings.Join(args, " "), "--simulate") {
			fmt.Fprintln(os.Stderr, "probe must use --simulate")
			os.Exit(1)
		}
		fmt.Println(helperInfo(mode))
		return
	}

	switch mode {
	case "ytdlp_meta_only":
		fmt.Fprintln(os.Stderr, "ERROR: unable to download video data: HTTP Error 403")
		os.Exit(1)

	case "ytdlp_success", "ytdlp_long":
		// Find output directory from -o flag
		var outTmpl string
		for i, a := range args {
			if a == "-o" && i+1 < len(args) {
				outTmpl = args[i+1]
				break
			}
		}
		if outTmpl == "" {
			fmt.Fprintln(os.Stderr, "no -o flag provided")
			os.Exit(1)
		}
		dir := filepath.Dir(outTmpl)

		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--ignore-config") {
			fmt.Fprintln(os.Stderr, "missing --ignore-config")
			os.Exit(1)
		}
		if !strings.Contains(joined, "--use-extractors") {
			fmt.Fprintln(os.Stderr, "missing --use-extractors")
			os.Exit(1)
		}
		if !strings.Contains(joined, " -- ") {
			fmt.Fprintln(os.Stderr, "missing -- separator")
			os.Exit(1)
		}

		// Write v.info.json
		infoJSON := `{
			"id": "1234567890",
			"title": "Uji Coba Video Hoaks",
			"description": "Deskripsi singkat video media sosial untuk verifikasi.",
			"uploader": "akun_resmi",
			"upload_date": "20261109",
			"duration": 45.5,
			"view_count": 10500,
			"width": 720,
			"height": 1280,
			"is_ai_generated": true
		}`
		if mode == "ytdlp_long" {
			infoJSON = helperInfo(mode)
		}
		_ = os.WriteFile(filepath.Join(dir, "v.info.json"), []byte(infoJSON), 0644)

		// Write mock v.mp4
		_ = os.WriteFile(filepath.Join(dir, "v.mp4"), []byte("FAKE_MP4_CONTENT_BYTES"), 0644)

		// Write mock .vtt subtitles
		vttData := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:03.000\nHalo ini adalah berita hoaks\n\n2\n00:00:03.000 --> 00:00:05.000\nHalo ini adalah berita hoaks\n\n3\n00:00:05.000 --> 00:00:08.000\nJangan percaya bantuan ini\n"
		_ = os.WriteFile(filepath.Join(dir, "v.id.vtt"), []byte(vttData), 0644)

	case "ytdlp_private":
		fmt.Fprintln(os.Stderr, "ERROR: Private video. Sign in if you've been granted access to this video")
		os.Exit(1)

	case "ytdlp_too_long":
		fmt.Fprintln(os.Stderr, "ERROR: [download] does not pass filter: duration <= 180")
		os.Exit(1)

	case "ytdlp_too_big":
		fmt.Fprintln(os.Stderr, "ERROR: [download] File is larger than max-filesize (62914560 bytes)")
		os.Exit(1)

	case "ytdlp_unsupported":
		fmt.Fprintln(os.Stderr, "ERROR: Unsupported URL: https://evil.com/video")
		os.Exit(1)

	case "ffmpeg_success":
		isAudio := false
		var outPath string
		for i, a := range args {
			if a == "-vn" {
				isAudio = true
			}
			if a == "-y" && i+1 < len(args) {
				outPath = args[i+1]
			}
		}
		if outPath != "" {
			if isAudio {
				_ = os.WriteFile(outPath, []byte("MOCK_MP3_AUDIO_BYTES_16KHZ"), 0644)
			} else {
				fakeJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xFF, 0xD9}
				_ = os.WriteFile(outPath, fakeJPEG, 0644)
			}
		}

	default:
		fmt.Fprintln(os.Stderr, "unknown helper mode:", mode)
		os.Exit(2)
	}
}

func helperCommand(mode string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		testArgs := []string{"-test.run=TestHelperProcess", "--", "--simulated-args"}
		testArgs = append(testArgs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], testArgs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_MODE="+mode,
		)
		return cmd
	}
}

func TestPlatform(t *testing.T) {
	tests := []struct {
		url          string
		wantPlatform string
		wantOK       bool
	}{
		// Valid cases
		{"https://www.tiktok.com/@user/video/7123456789", "tiktok", true},
		{"https://vt.tiktok.com/ZS12345/", "tiktok", true},
		{"https://vm.tiktok.com/ZM12345/", "tiktok", true},
		{"https://m.tiktok.com/v/7123456789.html", "tiktok", true},
		{"https://www.youtube.com/shorts/dQw4w9WgXcQ", "youtube", true},
		{"https://m.youtube.com/shorts/dQw4w9WgXcQ", "youtube", true},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube", true},
		{"https://youtu.be/dQw4w9WgXcQ", "youtube", true},
		{"https://www.instagram.com/reel/C-12345abc/", "instagram", true},
		{"https://instagram.com/reels/C-12345abc/", "instagram", true},
		{"https://www.instagram.com/p/C-12345abc/", "instagram", true},
		{"https://x.com/turnbackhoax/status/123456789012345", "x", true},
		{"https://twitter.com/turnbackhoax/status/123456789012345", "x", true},
		{"https://www.facebook.com/watch/?v=123456789", "facebook", true},
		{"https://www.facebook.com/reel/123456789", "facebook", true},
		{"https://fb.watch/abc123xyz/", "facebook", true},

		// Reject cases: non-https
		{"http://www.tiktok.com/@user/video/7123456789", "", false},
		{"http://youtu.be/dQw4w9WgXcQ", "", false},

		// Reject: userinfo
		{"https://tiktok.com@evil.com/video/123", "", false},
		{"https://user:pass@www.tiktok.com/@user/video/123", "", false},

		// Reject: explicit port
		{"https://www.tiktok.com:8443/@user/video/123", "", false},
		{"https://youtube.com:443/shorts/123", "", false},

		// Reject: IP literal
		{"https://192.168.1.1/video/123", "", false},
		{"https://[::1]/video/123", "", false},

		// Reject: lookalike hosts
		{"https://tiktok.com.evil.com/@user/video/123", "", false},
		{"https://fakeyoutube.com/shorts/123", "", false},
		{"https://fakex.com/user/status/123", "", false},

		// Reject: lookalikes and tricks
		{"https://tlktok.com/@a/video/1", "", false},
		{"https://www.tiktok.com@evil.com/video/1", "", false},
		{"https://tiktok.com.evil.xyz/@a/video/1", "", false},
		{"https://youtube.com.evil.xyz/shorts/abc", "", false},
		{"https://tіktok.com/@a/video/1", "", false},
		{"https://xn--tktok-n2e.com/@a/video/1", "", false},
		{"https://youtu.be.evil.com/abc", "", false},
		{"javascript:alert(1)", "", false},

		// Reject: unsupported paths on valid hosts
		{"https://www.youtube.com/channel/UC1234567890", "", false},
		{"https://www.youtube.com/feed/trending", "", false},
		{"https://www.instagram.com/explore/", "", false},
		{"https://x.com/home", "", false},
		{"https://www.facebook.com/home.php", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			plat, ok := Platform(tc.url)
			if ok != tc.wantOK || plat != tc.wantPlatform {
				t.Errorf("Platform(%q) = (%q, %v); want (%q, %v)", tc.url, plat, ok, tc.wantPlatform, tc.wantOK)
			}
		})
	}
}

func TestFetchSuccess(t *testing.T) {
	f := &Fetcher{
		YtDlp:        "fake-ytdlp",
		FFmpeg:       "fake-ffmpeg",
		MaxDuration:  180 * time.Second,
		MaxBytes:     60 * 1024 * 1024,
		Frames:       8,
		FrameMaxSide: 768,
		AudioMaxSec:  90,
		cmdRunner: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			if strings.Contains(name, "ytdlp") {
				return helperCommand("ytdlp_success")(ctx, name, args...)
			}
			return helperCommand("ffmpeg_success")(ctx, name, args...)
		},
	}

	clip, err := f.Fetch(context.Background(), "https://www.tiktok.com/@user/video/7123456789")
	if err != nil {
		t.Fatalf("Fetch unexpected error: %v", err)
	}

	if clip.Platform != "tiktok" {
		t.Errorf("want platform tiktok, got %q", clip.Platform)
	}
	if clip.ID != "1234567890" {
		t.Errorf("want ID 1234567890, got %q", clip.ID)
	}
	if !clip.AILabel {
		t.Errorf("want AILabel true, got false")
	}
	if clip.DurationSec != 45.5 {
		t.Errorf("want duration 45.5, got %v", clip.DurationSec)
	}
	if !strings.Contains(clip.Subtitles, "berita hoaks") {
		t.Errorf("subtitles missing expected text: %q", clip.Subtitles)
	}
	if strings.Count(clip.Subtitles, "Halo ini adalah berita hoaks") != 1 {
		t.Errorf("subtitles duplicate not stripped: %q", clip.Subtitles)
	}
	if len(clip.Frames) != 8 {
		t.Errorf("want 8 frames, got %d", len(clip.Frames))
	}
	if len(clip.Audio) == 0 {
		t.Errorf("expected audio bytes, got nil/empty")
	}
}

func TestFetchErrorMapping(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		url     string
		wantErr error
	}{
		{"private", "ytdlp_private", "https://www.tiktok.com/@user/video/1", ErrPrivate},
		{"too_long", "ytdlp_too_long", "https://www.tiktok.com/@user/video/1", ErrTooLong},
		{"too_big", "ytdlp_too_big", "https://www.tiktok.com/@user/video/1", ErrTooBig},
		{"unsupported", "ytdlp_unsupported", "https://www.tiktok.com/@user/video/1", ErrUnsupported},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &Fetcher{
				YtDlp:       "fake-ytdlp",
				FFmpeg:      "fake-ffmpeg",
				MaxDuration: 180 * time.Second,
				MaxBytes:    60 * 1024 * 1024,
				cmdRunner:   helperCommand(tc.mode),
			}
			_, err := f.Fetch(context.Background(), tc.url)
			if err != tc.wantErr {
				t.Errorf("got error %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestFetchUnsupportedURL(t *testing.T) {
	f := &Fetcher{YtDlp: "fake", FFmpeg: "fake"}
	_, err := f.Fetch(context.Background(), "https://not-allowed.example.com/video")
	if err != ErrUnsupported {
		t.Errorf("got %v, want ErrUnsupported", err)
	}
}

func TestFetchNoTools(t *testing.T) {
	f := &Fetcher{}
	_, err := f.Fetch(context.Background(), "https://www.tiktok.com/@user/video/1")
	if err != ErrNoTools {
		t.Errorf("got %v, want ErrNoTools", err)
	}
}

func TestFetchTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	mapped := mapYtDlpError(ctx, context.Canceled, "")
	if mapped != ErrTimeout {
		t.Errorf("got %v, want ErrTimeout", mapped)
	}
}

func TestLiveFetchOptIn(t *testing.T) {
	if os.Getenv("AMBA_VIDEO_LIVE") != "1" {
		t.Skip("skipping live test; set AMBA_VIDEO_LIVE=1 to run")
	}
	testURL := os.Getenv("AMBA_VIDEO_TEST_URL")
	if testURL == "" {
		t.Skip("AMBA_VIDEO_TEST_URL not set")
	}

	f := NewFromPath()
	if !f.Enabled() {
		t.Skip("yt-dlp or ffmpeg missing on host")
	}

	clip, err := f.Fetch(context.Background(), testURL)
	if err != nil {
		t.Fatalf("live fetch error: %v", err)
	}
	if clip.ID == "" {
		t.Errorf("empty clip ID")
	}
}

// helperInfo is what the fake yt-dlp prints for the metadata call.
func helperInfo(mode string) string {
	dur := "45.5"
	if mode == "ytdlp_long" || mode == "ytdlp_meta_only" {
		dur = "560"
	}
	return `{"id":"abc","title":"Sora 2 AI Animal Videos Compilation","description":"kompilasi","uploader":"Kanal Uji","uploader_id":"@kanaluji","tags":["sora"],"upload_date":"20261109","duration":` + dur + `,"width":720,"height":1280}`
}

func fetcherWith(mode string, log *[]string) *Fetcher {
	return &Fetcher{
		YtDlp: "fake-ytdlp", FFmpeg: "fake-ffmpeg", MaxDuration: 30 * time.Minute, MaxBytes: 60 << 20,
		Frames: 4, FrameMaxSide: 768, AudioMaxSec: 90,
		cmdRunner: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			if log != nil && strings.Contains(name, "ytdlp") {
				*log = append(*log, strings.Join(args, " "))
			}
			if strings.Contains(name, "ytdlp") {
				return helperCommand(mode)(ctx, name, args...)
			}
			return helperCommand("ffmpeg_success")(ctx, name, args...)
		},
	}
}

// A 560 s compilation: metadata comes first, then only the first 90 s are
// downloaded at 480p or less.
func TestLongVideoIsSampledAtLowResolution(t *testing.T) {
	var calls []string
	f := fetcherWith("ytdlp_long", &calls)
	clip, err := f.Fetch(context.Background(), "https://www.youtube.com/shorts/abcdefghijk")
	if err != nil || clip == nil || len(clip.Frames) == 0 {
		t.Fatalf("clip=%v err=%v", clip, err)
	}
	if len(calls) != 2 || !strings.Contains(calls[0], "--simulate") || !strings.Contains(calls[0], "--dump-json") {
		t.Fatalf("calls = %q", calls)
	}
	dl := calls[1]
	for _, want := range []string{"--download-sections *0-90", "height<=480"} {
		if !strings.Contains(dl, want) {
			t.Errorf("download lacks %q: %s", want, dl)
		}
	}
	if strings.Contains(dl, "height<=720") {
		t.Errorf("download must not ask for 720p: %s", dl)
	}
	if clip.DurationSec != 560 || clip.UploaderID != "@kanaluji" || clip.Title == "" {
		t.Fatalf("clip = %+v", clip)
	}
	// A short clip is downloaded whole.
	calls = nil
	if _, err := fetcherWith("ytdlp_success", &calls).Fetch(context.Background(), "https://www.tiktok.com/@user/video/7123456789"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(calls[len(calls)-1], "--download-sections") {
		t.Errorf("short clip must not be cut: %s", calls[len(calls)-1])
	}
}

// When the download fails the metadata is kept: the title still tells the
// creator's disclosure and feeds the search.
func TestFailedDownloadKeepsTheMetadata(t *testing.T) {
	clip, err := fetcherWith("ytdlp_meta_only", nil).Fetch(context.Background(), "https://www.youtube.com/shorts/abcdefghijk")
	if err != ErrDownload {
		t.Fatalf("err = %v, want ErrDownload", err)
	}
	if clip == nil || len(clip.Frames) != 0 || clip.Title != "Sora 2 AI Animal Videos Compilation" || clip.Uploader != "Kanal Uji" {
		t.Fatalf("clip = %+v", clip)
	}
}

// A dead probe is not final: the download is still tried and gives the answer.
func TestProbeFailureFallsBackToTheDownload(t *testing.T) {
	var calls []string
	f := fetcherWith("ytdlp_probe_fail", &calls)
	clip, err := f.Fetch(context.Background(), "https://www.tiktok.com/@user/video/1")
	if clip != nil || err == nil || len(calls) != 2 {
		t.Fatalf("clip=%v err=%v calls=%d", clip, err, len(calls))
	}
}

func TestTooLongSourceIsRefusedBeforeDownload(t *testing.T) {
	var calls []string
	f := fetcherWith("ytdlp_long", &calls)
	f.MaxDuration = 300 * time.Second
	_, err := f.Fetch(context.Background(), "https://www.youtube.com/shorts/abcdefghijk")
	if err != ErrTooLong || len(calls) != 1 {
		t.Fatalf("err=%v calls=%d", err, len(calls))
	}
}
