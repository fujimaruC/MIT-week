package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/amba/amba/backend/internal/video"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <social-video-url>\n", os.Args[0])
		os.Exit(1)
	}

	rawURL := os.Args[1]
	fetcher := video.NewFromPath()
	if !fetcher.Enabled() {
		fmt.Fprintf(os.Stderr, "error: yt-dlp or ffmpeg not found in PATH\n")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	start := time.Now()
	clip, err := fetcher.Fetch(ctx, rawURL)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch failed: %v (elapsed: %d ms)\n", err, elapsed.Milliseconds())
		os.Exit(3)
	}

	fmt.Printf("platform: %s\n", clip.Platform)
	fmt.Printf("id: %s\n", clip.ID)
	fmt.Printf("duration_sec: %.2f\n", clip.DurationSec)
	fmt.Printf("frames: %d\n", len(clip.Frames))
	fmt.Printf("subtitle_chars: %d\n", len([]rune(clip.Subtitles)))
	fmt.Printf("audio_bytes: %d\n", len(clip.Audio))
	fmt.Printf("ai_label: %v\n", clip.AILabel)
	fmt.Printf("elapsed_ms: %d\n", elapsed.Milliseconds())
}
