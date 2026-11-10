//go:build !unix

package video

import "os/exec"

// killProcessGroup is a no-op where process groups are not available; the
// WaitDelay in boundedCommand still bounds Wait.
func killProcessGroup(*exec.Cmd) {}
