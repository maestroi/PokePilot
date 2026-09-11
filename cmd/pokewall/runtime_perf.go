package main

import (
	"context"
	"sync"
	"time"
)

// Heartbeats are high-frequency telemetry, not lifecycle transitions. Persist
// the first heartbeat of a generation immediately, then at most once per
// interval while it remains active. Lease/finish/delete paths still call
// saveState synchronously. This preserves restart semantics while avoiding a
// complete historical-catalog rewrite every second.
const heartbeatStateFlushInterval = 5 * time.Second

type persistenceCoordinator struct {
	writeMu sync.Mutex

	heartbeatMu   sync.Mutex
	lastHeartbeat time.Time
}

var persistenceCoordinators sync.Map // *Wall -> *persistenceCoordinator

func persistenceFor(w *Wall) *persistenceCoordinator {
	if w == nil {
		return &persistenceCoordinator{}
	}
	if existing, ok := persistenceCoordinators.Load(w); ok {
		return existing.(*persistenceCoordinator)
	}
	created := &persistenceCoordinator{}
	actual, _ := persistenceCoordinators.LoadOrStore(w, created)
	return actual.(*persistenceCoordinator)
}

// saveHeartbeatState keeps the first running snapshot durable and throttles
// later telemetry-only saves. It deliberately performs no background work:
// tests and short-lived wall processes cannot leave a timer trying to write a
// state directory after it has been removed.
func (w *Wall) saveHeartbeatState(force bool) {
	if w == nil || w.statePath == "" {
		return
	}
	p := persistenceFor(w)
	now := time.Now()
	p.heartbeatMu.Lock()
	due := force || p.lastHeartbeat.IsZero() || now.Sub(p.lastHeartbeat) >= heartbeatStateFlushInterval
	if due {
		p.lastHeartbeat = now
	}
	p.heartbeatMu.Unlock()
	if due {
		w.saveState()
	}
}

// serializeStateWrite ensures snapshots reach disk in the same order they are
// taken. Without this, two saveState calls can marshal A then B but finish
// their atomic renames B then A, resurrecting stale queue/run state.
func (w *Wall) serializeStateWrite(fn func()) {
	p := persistenceFor(w)
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	fn()
}

const liveFrameFreshFor = 50 * time.Millisecond

type frameCacheEntry struct {
	data []byte
	at   time.Time
}

type frameFetchCall struct {
	done chan struct{}
	data []byte
	err  error
}

type frameCoordinator struct {
	mu       sync.Mutex
	cache    map[string]frameCacheEntry
	inflight map[string]*frameFetchCall
}

var frameCoordinators sync.Map // *Wall -> *frameCoordinator

func framesFor(w *Wall) *frameCoordinator {
	if existing, ok := frameCoordinators.Load(w); ok {
		return existing.(*frameCoordinator)
	}
	created := &frameCoordinator{
		cache:    make(map[string]frameCacheEntry),
		inflight: make(map[string]*frameFetchCall),
	}
	actual, _ := frameCoordinators.LoadOrStore(w, created)
	return actual.(*frameCoordinator)
}

// fetchRunnerFrameCached caps one run to roughly the browser's 20 fps and
// collapses concurrent viewers/publisher reads onto a single upstream fetch.
func (w *Wall) fetchRunnerFrameCached(ctx context.Context, runID string, addrs []string) ([]byte, error) {
	frames := framesFor(w)
	now := time.Now()
	frames.mu.Lock()
	if cached, ok := frames.cache[runID]; ok && len(cached.data) > 0 && now.Sub(cached.at) < liveFrameFreshFor {
		data := cached.data
		frames.mu.Unlock()
		return data, nil
	}
	if call := frames.inflight[runID]; call != nil {
		done := call.done
		frames.mu.Unlock()
		select {
		case <-done:
			return call.data, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &frameFetchCall{done: make(chan struct{})}
	frames.inflight[runID] = call
	frames.mu.Unlock()

	data, err := fetchRunnerFrame(addrs)

	frames.mu.Lock()
	call.data, call.err = data, err
	if err == nil && len(data) > 0 {
		frames.cache[runID] = frameCacheEntry{data: data, at: time.Now()}
	}
	delete(frames.inflight, runID)
	close(call.done)
	frames.mu.Unlock()
	return data, err
}

func (w *Wall) rememberFrame(runID string, data []byte) {
	if len(data) == 0 {
		return
	}
	frames := framesFor(w)
	frames.mu.Lock()
	frames.cache[runID] = frameCacheEntry{data: data, at: time.Now()}
	frames.mu.Unlock()
}

func (w *Wall) dropFrameCache(runID string) {
	frames := framesFor(w)
	frames.mu.Lock()
	delete(frames.cache, runID)
	frames.mu.Unlock()
}
