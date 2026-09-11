package main

import (
	"context"
	"sync"
	"time"
)

const (
	maxConcurrentReplayRenders = 1
	maxReplayProcessLogBytes    = 64 << 10
	replayJobRetention          = 30 * time.Minute
)

var replayRenderSlots = make(chan struct{}, maxConcurrentReplayRenders)

// acquireReplayRender bounds expensive emulator+encoder processes. One slot is
// intentional: VAAPI is a shared device and CPU fallback renders are expensive
// enough that concurrent jobs are more likely to make both slower than improve
// throughput. Additional requests remain lightweight waiting goroutines.
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
