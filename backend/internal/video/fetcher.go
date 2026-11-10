package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultMaxDuration  = 30 * time.Minute
	defaultMaxBytes     = 60 * 1024 * 1024 // 60 MB
	defaultFrames       = 8
	defaultFrameMaxSide = 768
	defaultAudioMaxSec  = 90
	maxAudioBytes       = 1024 * 1024 // 1 MB
	maxTitleRunes       = 300
	maxDescRunes        = 2000
	maxSubtitleRunes    = 4000
)

// NewFromPath searches PATH for yt-dlp and ffmpeg and populates standard defaults.
func NewFromPath() *Fetcher {
	ytDlp, _ := exec.LookPath("yt-dlp")
	ffmpeg, _ := exec.LookPath("ffmpeg")
	return &Fetcher{
		YtDlp:        ytDlp,
		FFmpeg:       ffmpeg,
		MaxDuration:  defaultMaxDuration,
		MaxBytes:     defaultMaxBytes,
		Frames:       defaultFrames,
		FrameMaxSide: defaultFrameMaxSide,
		AudioMaxSec:  defaultAudioMaxSec,
	}
}

// Enabled returns true if both yt-dlp and ffmpeg paths are configured.
func (f *Fetcher) Enabled() bool {
	return f != nil && f.YtDlp != "" && f.FFmpeg != ""
}

// ytdlpInfoJSON matches standard yt-dlp metadata fields.
type ytdlpInfoJSON struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Uploader    string   `json:"uploader"`
	Channel     string   `json:"channel"`
	UploaderID  string   `json:"uploader_id"`
	ChannelID   string   `json:"channel_id"`
	Tags        []string `json:"tags"`
	UploadDate  string   `json:"upload_date"`
	Duration    float64  `json:"duration"`
	ViewCount   int64    `json:"view_count"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`

	// Platform AI-generated flags
	AILabel       bool `json:"is_ai_generated"`
	AIGenerated   bool `json:"ai_generated"`
	TikTokAILabel bool `json:"is_synthetic"`
	TikTokAITag   bool `json:"aigc_label_type"`
}

// Fetch reads a social video's metadata with a fast call, then downloads it
// at 480p or less (only its first 90 s when it runs over 120 s), and extracts
// frames and audio.
//
// When the metadata is known but the download or the frame extraction fails,
// Fetch returns the metadata-only Clip (no Frames) together with the error, so
// the caller can still read the title, the creator's disclosure and the
// description. A nil Clip means nothing was learned.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Clip, error) {
	if !f.Enabled() {
		return nil, ErrNoTools
	}

	platform, ok := Platform(rawURL)
	if !ok {
		return nil, ErrUnsupported
	}

	extractor := ExtractorForPlatform(platform)
	if extractor == "" {
		return nil, ErrUnsupported
	}

	start := time.Now()

	maxDurSec := int(f.MaxDuration.Seconds())
	if maxDurSec <= 0 {
		maxDurSec = int(defaultMaxDuration.Seconds())
	}
	maxBytes := f.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}

	// Step 1: metadata only. A failure here is not final: the download below
	// reads the same metadata and gives the definite error.
	meta, metaErr := f.probe(ctx, extractor, rawURL)
	var metaInfo *ytdlpInfoJSON
	var metaRaw []byte
	if metaErr == nil {
		metaInfo, metaRaw = meta.info, meta.raw
		if time.Duration(metaInfo.Duration*float64(time.Second)) > f.maxDuration() {
			return nil, ErrTooLong
		}
	} else if errors.Is(metaErr, ErrPrivate) || errors.Is(metaErr, ErrUnsupported) || errors.Is(metaErr, ErrTooLong) {
		return nil, metaErr
	}
	// partial is what a failed download still returns.
	partial := func(err error) (*Clip, error) {
		if metaInfo == nil {
			return nil, err
		}
		return f.clipFrom(platform, metaInfo, metaRaw, "", nil, nil), err
	}

	// Create temp directory for this fetch
	tmpDir, err := os.MkdirTemp("", "amba-video-*")
	if err != nil {
		return partial(fmt.Errorf("video: create temp dir: %w", err))
	}
	defer os.RemoveAll(tmpDir)

	// Prepare output template
	outputTemplate := filepath.Join(tmpDir, "v.%(ext)s")

	// yt-dlp arguments (strict allowlist, no generic extractor, -i ignores subtitle 429s)
	args := []string{
		"--ignore-config",
		"--no-playlist",
		"--no-warnings",
		"--no-progress",
		"--restrict-filenames",
		"--no-part",
		"--no-mtime",
		"--socket-timeout", "10",
		"--retries", "1",
		"-i",
		"--use-extractors", extractor,
		"--max-filesize", fmt.Sprintf("%d", maxBytes),
		"--match-filter", fmt.Sprintf("duration<=%d", maxDurSec),
		"-f", downloadFormat,
		"--merge-output-format", "mp4",
		"--write-info-json",
		"--write-subs",
		"--write-auto-subs",
		"--sub-langs", "id.*,en.*",
		"--sub-format", "vtt",
	}
	// A long video is only sampled: its first 90 s say enough, and the whole
	// download would overrun the fetch deadline (10 Nov: a 560 s compilation).
	analyzed := 0.0
	if metaInfo != nil && metaInfo.Duration > longVideoSec {
		args = append(args, "--download-sections", fmt.Sprintf("*0-%d", sampleSec))
		analyzed = sampleSec
	}
	args = append(args, "-o", outputTemplate, "--", rawURL)

	cmd := f.command(ctx, f.YtDlp, args...)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	cmdErr := cmd.Run()

	// Locate info.json
	infoPath := filepath.Join(tmpDir, "v.info.json")
	infoData, readInfoErr := os.ReadFile(infoPath)
	if readInfoErr != nil {
		matches, _ := filepath.Glob(filepath.Join(tmpDir, "*.info.json"))
		if len(matches) > 0 {
			infoData, readInfoErr = os.ReadFile(matches[0])
		}
	}

	// Locate video file (v.mp4 or other extension)
	videoFile := filepath.Join(tmpDir, "v.mp4")
	if _, statErr := os.Stat(videoFile); os.IsNotExist(statErr) {
		candidates, _ := filepath.Glob(filepath.Join(tmpDir, "v.*"))
		found := ""
		for _, c := range candidates {
			ext := strings.ToLower(filepath.Ext(c))
			if ext != ".json" && ext != ".vtt" && ext != ".srt" {
				found = c
				break
			}
		}
		if found != "" {
			videoFile = found
		} else {
			videoFile = ""
		}
	}

	// A failed download that left no file: map the error and keep the metadata.
	if videoFile == "" || (readInfoErr != nil && metaInfo == nil) {
		mappedErr := mapYtDlpError(ctx, cmdErr, stderrBuf.String())
		slog.Warn("video fetch failed",
			"platform", platform,
			"duration_ms", time.Since(start).Milliseconds(),
			"error_category", mappedErr.Error(),
			"metadata_kept", metaInfo != nil,
		)
		if errors.Is(mappedErr, ErrPrivate) || errors.Is(mappedErr, ErrUnsupported) || errors.Is(mappedErr, ErrTooLong) {
			return nil, mappedErr
		}
		return partial(mappedErr)
	}

	// The download's own metadata is the more complete; the probe's will do.
	var info ytdlpInfoJSON
	if readInfoErr == nil && json.Unmarshal(infoData, &info) == nil {
		// keep
	} else if metaInfo != nil {
		info, infoData = *metaInfo, metaRaw
	} else {
		return nil, ErrDownload
	}

	// Verify duration limit from metadata
	if time.Duration(info.Duration*float64(time.Second)) > f.maxDuration() {
		return nil, ErrTooLong
	}
	frameSpan := info.Duration
	if analyzed > 0 {
		frameSpan = analyzed
	}
	if analyzed > 0 {
		frameSpan = analyzed
	}

	// Read subtitles if available
	subtitles := loadSubtitles(tmpDir)

	// Extract keyframes using ffmpeg
	framesCount := f.Frames
	if framesCount <= 0 {
		framesCount = defaultFrames
	}
	frameMaxSide := f.FrameMaxSide
	if frameMaxSide <= 0 {
		frameMaxSide = defaultFrameMaxSide
	}

	frames, err := f.extractFrames(ctx, videoFile, frameSpan, framesCount, frameMaxSide, tmpDir)
	if err != nil {
		slog.Warn("video keyframe extraction failed", "platform", platform, "error", err.Error())
	}
	if len(frames) == 0 {
		return f.clipFromOrMeta(platform, &info, infoData, metaInfo, metaRaw), ErrDownload
	}

	// Extract audio (optional)
	audioMaxSec := f.AudioMaxSec
	if audioMaxSec <= 0 {
		audioMaxSec = defaultAudioMaxSec
	}
	audioBytes := f.extractAudio(ctx, videoFile, audioMaxSec, tmpDir)

	clip := f.clipFrom(platform, &info, infoData, subtitles, frames, audioBytes)

	slog.Info("video fetched",
		"platform", platform,
		"duration_sec", info.Duration,
		"sampled_sec", analyzed,
		"frames", len(frames),
		"audio_bytes", len(audioBytes),
		"elapsed_ms", time.Since(start).Milliseconds(),
	)

	return clip, nil
}

const (
	// downloadFormat keeps the download small and fast: 480p or less.
	downloadFormat = "bestvideo[height<=480][ext=mp4]+bestaudio[ext=m4a]/best[height<=480][ext=mp4]/best[height<=480]/worst"
	// longVideoSec is the length over which only the first sampleSec seconds
	// are downloaded.
	longVideoSec = 120
	sampleSec    = 90
	// probeBudget bounds the metadata call.
	probeBudget = 8 * time.Second
)

func (f *Fetcher) maxDuration() time.Duration {
	if f.MaxDuration > 0 {
		return f.MaxDuration
	}
	return defaultMaxDuration
}

func (f *Fetcher) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if f.cmdRunner != nil {
		return f.cmdRunner(ctx, name, args...)
	}
	return boundedCommand(ctx, name, args...)
}

// commandWaitDelay bounds how long Wait may block after ctx ends.
const commandWaitDelay = 3 * time.Second

// boundedCommand is exec.CommandContext that cannot outlive ctx by more than
// commandWaitDelay. yt-dlp starts ffmpeg for merges and sections; when ctx
// ends only yt-dlp was killed, ffmpeg kept the stderr pipe open and Wait
// blocked until it exited (10 Nov: a 484 s video answered after 117 s, past
// the server's write timeout, so nginx served a 502). The whole process group
// is killed where the platform allows it, and WaitDelay caps the rest.
func boundedCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	killProcessGroup(cmd)
	cmd.WaitDelay = commandWaitDelay
	return cmd
}

type probeResult struct {
	info *ytdlpInfoJSON
	raw  []byte
}

// probe reads the post's metadata without downloading anything
// (yt-dlp --simulate --dump-json), within probeBudget.
func (f *Fetcher) probe(ctx context.Context, extractor, rawURL string) (probeResult, error) {
	pctx, cancel := context.WithTimeout(ctx, probeBudget)
	defer cancel()
	cmd := f.command(pctx, f.YtDlp,
		"--ignore-config", "--no-playlist", "--no-warnings", "--no-progress",
		"--socket-timeout", "8", "--retries", "1",
		"--use-extractors", extractor,
		"--simulate", "--dump-json",
		"--", rawURL)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == nil && pctx.Err() != nil {
			return probeResult{}, ErrTimeout
		}
		return probeResult{}, mapYtDlpError(ctx, err, stderr.String())
	}
	raw := bytes.TrimSpace(stdout.Bytes())
	// --dump-json prints one JSON object per line; the first is the post.
	if i := bytes.IndexByte(raw, '\n'); i > 0 {
		raw = raw[:i]
	}
	var info ytdlpInfoJSON
	if err := json.Unmarshal(raw, &info); err != nil {
		return probeResult{}, ErrDownload
	}
	return probeResult{info: &info, raw: raw}, nil
}

// clipFrom builds the Clip from metadata; frames and audio may be empty.
func (f *Fetcher) clipFrom(platform string, info *ytdlpInfoJSON, raw []byte, subtitles string, frames []Frame, audio []byte) *Clip {
	return &Clip{
		Platform:    platform,
		ID:          info.ID,
		Title:       truncateRunes(info.Title, maxTitleRunes),
		Description: truncateRunes(info.Description, maxDescRunes),
		Uploader:    firstNonEmpty(info.Uploader, info.Channel),
		UploaderID:  truncateRunes(firstNonEmpty(info.UploaderID, info.ChannelID), 80),
		Tags:        capTags(info.Tags),
		UploadDate:  info.UploadDate,
		DurationSec: info.Duration,
		ViewCount:   info.ViewCount,
		Width:       info.Width,
		Height:      info.Height,
		AILabel:     detectAILabel(raw, info),
		Subtitles:   subtitles,
		Frames:      frames,
		Audio:       audio,
	}
}

// clipFromOrMeta is the metadata-only clip after a failed frame extraction:
// the download's own metadata, or the probe's when that failed to parse.
func (f *Fetcher) clipFromOrMeta(platform string, info *ytdlpInfoJSON, raw []byte, metaInfo *ytdlpInfoJSON, metaRaw []byte) *Clip {
	if info != nil {
		return f.clipFrom(platform, info, raw, "", nil, nil)
	}
	if metaInfo != nil {
		return f.clipFrom(platform, metaInfo, metaRaw, "", nil, nil)
	}
	return nil
}

func detectAILabel(rawJSON []byte, info *ytdlpInfoJSON) bool {
	if info.AILabel || info.AIGenerated || info.TikTokAILabel || info.TikTokAITag {
		return true
	}
	lowerJSON := strings.ToLower(string(rawJSON))
	patterns := []string{
		`"aigc_label_type":1`,
		`"is_ai_generated":true`,
		`"is_synthetic":true`,
		`"aigc_info":`,
	}
	for _, p := range patterns {
		if strings.Contains(lowerJSON, p) {
			return true
		}
	}
	return false
}

func loadSubtitles(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.vtt"))
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err == nil && len(data) > 0 {
			cleaned := cleanVTT(data, maxSubtitleRunes)
			if cleaned != "" {
				return cleaned
			}
		}
	}
	return ""
}

func (f *Fetcher) extractFrames(ctx context.Context, videoFile string, duration float64, count, maxSide int, tmpDir string) ([]Frame, error) {
	if count <= 0 {
		return nil, nil
	}

	if duration <= 0 {
		duration = 10.0
	}

	var timestamps []float64
	if count == 1 {
		timestamps = []float64{duration * 0.5}
	} else {
		start := duration * 0.05
		end := duration * 0.95
		step := (end - start) / float64(count-1)
		for i := 0; i < count; i++ {
			ts := start + float64(i)*step
			if ts > duration {
				ts = duration
			}
			timestamps = append(timestamps, math.Round(ts*100)/100)
		}
	}

	scaleFilter := fmt.Sprintf("scale='if(gt(iw,ih),min(%d,iw),-2)':'if(gt(iw,ih),-2,min(%d,ih))'", maxSide, maxSide)

	var frames []Frame
	for i, ts := range timestamps {
		outFramePath := filepath.Join(tmpDir, fmt.Sprintf("frame_%02d.jpg", i))
		args := []string{
			"-ss", fmt.Sprintf("%.3f", ts),
			"-i", videoFile,
			"-vf", scaleFilter,
			"-vframes", "1",
			"-q:v", "3", // ~quality 80
			"-y",
			outFramePath,
		}

		cmd := f.command(ctx, f.FFmpeg, args...)
		if err := cmd.Run(); err != nil {
			continue
		}

		jpegData, err := os.ReadFile(outFramePath)
		if err == nil && len(jpegData) > 0 {
			frames = append(frames, Frame{
				AtSec: ts,
				JPEG:  jpegData,
			})
		}
	}

	return frames, nil
}

func (f *Fetcher) extractAudio(ctx context.Context, videoFile string, maxSec int, tmpDir string) []byte {
	outAudioPath := filepath.Join(tmpDir, "audio.mp3")
	args := []string{
		"-i", videoFile,
		"-t", fmt.Sprintf("%d", maxSec),
		"-vn",
		"-ac", "1", // mono
		"-ar", "16000", // 16 kHz
		"-b:a", "32k",
		"-y",
		outAudioPath,
	}

	cmd := f.command(ctx, f.FFmpeg, args...)
	if err := cmd.Run(); err != nil {
		return nil
	}

	audioData, err := os.ReadFile(outAudioPath)
	if err != nil || len(audioData) == 0 {
		return nil
	}

	if len(audioData) > maxAudioBytes {
		return audioData[:maxAudioBytes]
	}
	return audioData
}

func mapYtDlpError(ctx context.Context, cmdErr error, stderr string) error {
	if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
		return ErrTimeout
	}

	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "private video") ||
		strings.Contains(lower, "login required") ||
		strings.Contains(lower, "sign in") ||
		strings.Contains(lower, "requires authentication") ||
		strings.Contains(lower, "this video is only available to registered users") ||
		strings.Contains(lower, "video unavailable"):
		return ErrPrivate

	case strings.Contains(lower, "does not pass filter") ||
		(strings.Contains(lower, "duration") && strings.Contains(lower, "filter")):
		return ErrTooLong

	case strings.Contains(lower, "larger than max-filesize") ||
		strings.Contains(lower, "file is larger than"):
		return ErrTooBig

	case strings.Contains(lower, "unsupported url") ||
		strings.Contains(lower, "no suitable extractor") ||
		strings.Contains(lower, "not found"):
		return ErrUnsupported

	case strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout"):
		return ErrTimeout

	default:
		return ErrDownload
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// capTags keeps at most 30 tags of at most 60 runes each.
func capTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		if len(out) == 30 {
			break
		}
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, truncateRunes(t, 60))
		}
	}
	return out
}
