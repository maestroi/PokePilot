package encode

import (
	"context"
	"os/exec"
	"sync"
)

const DefaultMaxOutputBytes = 64 << 10

// Runner is the process boundary used by media encoders and compositors.
// Implementations can execute real binaries or provide deterministic fakes in
// package tests without constructing an HTTP server or replay worker.
type Runner interface {
	Run(context.Context, string, ...string) (string, error)
}

// ExecRunner executes a process while retaining only the newest bounded output.
type ExecRunner struct {
	MaxOutputBytes int
}

func NewExecRunner(maxOutputBytes int) ExecRunner {
	if maxOutputBytes <= 0 {
		maxOutputBytes = DefaultMaxOutputBytes
	}
	return ExecRunner{MaxOutputBytes: maxOutputBytes}
}

func (r ExecRunner) Run(ctx context.Context, binary string, args ...string) (string, error) {
	limit := r.MaxOutputBytes
	if limit <= 0 {
		limit = DefaultMaxOutputBytes
	}
	output := &tailWriter{limit: limit}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	return output.String(), err
}

type tailWriter struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	limit := w.limit
	if limit <= 0 {
		limit = DefaultMaxOutputBytes
	}
	if len(p) >= limit {
		w.buf = append(w.buf[:0], p[len(p)-limit:]...)
		return len(p), nil
	}
	if over := len(w.buf) + len(p) - limit; over > 0 {
		copy(w.buf, w.buf[over:])
		w.buf = w.buf[:len(w.buf)-over]
	}
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(append([]byte(nil), w.buf...))
}
