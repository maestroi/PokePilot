package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultReplayRenderWorkers = 3
	maxReplayProcessLogBytes   = 64 << 10
	replayJobRetention         = 30 * time.Minute
)

var replayRenderSlots = make(chan struct{}, replayRenderWorkers())

// replayRenderWorkers is how many attempt segments render at once across all
// jobs. Each segment is a single-threaded emulator piped into one encoder
// (~1k fps on the worker-05 iGPU node), so a long run is bound by segment
// count, not by the VAAPI engine. POKEPILOT_REPLAY_WORKERS tunes it per node.
func replayRenderWorkers() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("POKEPILOT_REPLAY_WORKERS"))); err == nil && n > 0 {
		return n
	}
	return defaultReplayRenderWorkers
}

// acquireReplayRender bounds expensive emulator+encoder processes. Additional
// segments remain lightweight waiting goroutines.
func acquireReplayRender(ctx context.Context) (func(), error) {
	select {
	case replayRenderSlots <- struct{}{}:
		return func() { <-replayRenderSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// replayOutputTail retains only the newest subprocess output. CombinedOutput
// used to retain an unbounded buffer for a command allowed to run for two
// hours; this keeps useful diagnostics without allowing encoder chatter to
// become a memory leak.
type replayOutputTail struct {
	mu  sync.Mutex
	buf []byte
}

func (w *replayOutputTail) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(p) >= maxReplayProcessLogBytes {
		w.buf = append(w.buf[:0], p[len(p)-maxReplayProcessLogBytes:]...)
		return len(p), nil
	}
	if over := len(w.buf) + len(p) - maxReplayProcessLogBytes; over > 0 {
		copy(w.buf, w.buf[over:])
		w.buf = w.buf[:len(w.buf)-over]
	}
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *replayOutputTail) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(append([]byte(nil), w.buf...))
}

func scheduleReplayJobExpiry(s *replayServer, key string, terminal replayStatus) {
	if s == nil || terminal.State == "generating" {
		return
	}
	time.AfterFunc(replayJobRetention, func() {
		s.mu.Lock()
		current, ok := s.jobs[key]
		if ok && current.State == terminal.State && current.ObjectKey == terminal.ObjectKey && current.Error == terminal.Error && current.Size == terminal.Size {
			delete(s.jobs, key)
		}
		s.mu.Unlock()
	})
}
