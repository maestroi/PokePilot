//go:build linux

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// configureReplayProcessGroup makes cancellation kill the entire encoder tree.
// gomeboy-stream may launch FFmpeg, so killing only the wrapper can otherwise
// leave an orphan encoder consuming GPU/CPU after a render is cancelled.
func configureReplayProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 5 * time.Second
}
