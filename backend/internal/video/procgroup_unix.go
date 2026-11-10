//go:build unix

package video

import (
	"os/exec"
	"syscall"
)

// killProcessGroup starts cmd in its own process group and makes the context
// cancel kill the whole group, so ffmpeg children of yt-dlp die with it.
func killProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
