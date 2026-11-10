package video

import (
	"bytes"
	"context"
	"runtime"
	"testing"
	"time"
)

// A child that keeps stderr open must not hold Run past the context (10 Nov:
// yt-dlp's ffmpeg kept a request alive for 117 s and nginx served a 502).
func TestBoundedCommandReturnsSoonAfterCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd := boundedCommand(ctx, "sh", "-c", "sleep 30 & sleep 30")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	start := time.Now()
	_ = cmd.Run()
	if d := time.Since(start); d > commandWaitDelay+2*time.Second {
		t.Fatalf("Run returned after %v; a child holding stderr kept it alive", d)
	}
}
