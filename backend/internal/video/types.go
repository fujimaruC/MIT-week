package video

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

// Sentinel errors mapped from fetch/probe operations.
var (
	ErrUnsupported = errors.New("video: unsupported URL or platform")
	ErrPrivate     = errors.New("video: post is private, deleted, or requires login")
	ErrTooLong     = errors.New("video: duration exceeds maximum allowed")
	ErrTooBig      = errors.New("video: file size exceeds maximum allowed")
	ErrTimeout     = errors.New("video: fetch operation timed out")
	ErrDownload    = errors.New("video: download failed")
	ErrNoTools     = errors.New("video: required tools (yt-dlp or ffmpeg) not available")
)

// Clip holds the downloaded video metadata, keyframes, subtitles, and optional audio.
type Clip struct {
	Platform    string   // tiktok, youtube, instagram, x, facebook
	ID          string   // platform-specific video ID
	Title       string   // title capped to 300 runes
	Description string   // description capped to 2000 runes
	Uploader    string   // channel/account name
	UploaderID  string   // account handle or id (yt-dlp "uploader_id"), "" when unknown
	Tags        []string // creator tags, capped (yt-dlp "tags")
	UploadDate  string   // YYYYMMDD
	DurationSec float64  // total duration in seconds
	ViewCount   int64    // view count if provided by platform
	Width       int      // video width in pixels
	Height      int      // video height in pixels
	AILabel     bool     // true only if platform metadata marks the post as AI-generated
	Subtitles   string   // plain text from .vtt (tags/timestamps stripped, deduplicated), <= 4000 runes
	Frames      []Frame  // extracted keyframes
	Audio       []byte   // optional mono 16 kHz mp3 of first AudioMaxSec, <= 1 MB; nil if unavailable
}

// Frame represents one extracted keyframe image.
type Frame struct {
	AtSec float64 // timestamp in seconds where frame was extracted
	JPEG  []byte  // JPEG-encoded image bytes
}

// Fetcher configures external binary invocation for social video download and extraction.
type Fetcher struct {
	YtDlp        string        // path to yt-dlp binary
	FFmpeg       string        // path to ffmpeg binary
	MaxDuration  time.Duration // default 180s
	MaxBytes     int64         // default 60 MB
	Frames       int           // default 8
	FrameMaxSide int           // default 768
	AudioMaxSec  int           // default 90

	// cmdRunner allows injecting command runner for tests (default exec.CommandContext)
	cmdRunner func(ctx context.Context, name string, arg ...string) *exec.Cmd
}
