package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	protocol "github.com/maestroi/pokepilot/renderstate"
)

const (
	maxRunnerRenderStateBytes = 4 << 20
	liveRenderStateFreshFor   = 100 * time.Millisecond
)

type renderStateCacheEntry struct {
	data []byte
	at   time.Time
}

type renderStateFetchCall struct {
	done chan struct{}
	data []byte
	err  error
}

type renderStateCoordinator struct {
	mu       sync.Mutex
	cache    map[string]renderStateCacheEntry
	inflight map[string]*renderStateFetchCall
}

var renderStateCoordinators sync.Map // *Wall -> *renderStateCoordinator

func renderStatesFor(w *Wall) *renderStateCoordinator {
	if existing, ok := renderStateCoordinators.Load(w); ok {
		return existing.(*renderStateCoordinator)
	}
	created := &renderStateCoordinator{
		cache:    make(map[string]renderStateCacheEntry),
		inflight: make(map[string]*renderStateFetchCall),
	}
	actual, _ := renderStateCoordinators.LoadOrStore(w, created)
	return actual.(*renderStateCoordinator)
}

func fetchRunnerRenderState(addrs []string) ([]byte, error) {
	for _, addr := range addrs {
		up, err := frameClient.Get("http://" + addr + "/render-state.json")
		if err != nil {
			continue
		}
		if up.StatusCode != http.StatusOK {
			up.Body.Close()
			continue
		}
		data, rerr := io.ReadAll(io.LimitReader(up.Body, maxRunnerRenderStateBytes+1))
		up.Body.Close()
		if rerr != nil || len(data) == 0 || len(data) > maxRunnerRenderStateBytes {
			continue
		}
		if _, err := protocol.ReadJSON(bytes.NewReader(data)); err != nil {
			continue
		}
		return data, nil
	}
	return nil, errors.New("runner render state unavailable")
}

// fetchRunnerRenderStateCached caps public/operator semantic-state fanout while
// coalescing concurrent viewers onto a single read of the runner's buffered
// snapshot. Unlike framebuffer playback this endpoint is point-in-time state,
// so reads do not drain any emulator-time queue.
func (w *Wall) fetchRunnerRenderStateCached(ctx context.Context, runID string, addrs []string) ([]byte, error) {
	states := renderStatesFor(w)
	now := time.Now()

	states.mu.Lock()
	if cached, ok := states.cache[runID]; ok && len(cached.data) > 0 && now.Sub(cached.at) < liveRenderStateFreshFor {
		data := cached.data
		states.mu.Unlock()
		return data, nil
	}
	if call := states.inflight[runID]; call != nil {
		done := call.done
		states.mu.Unlock()
		select {
		case <-done:
			return call.data, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &renderStateFetchCall{done: make(chan struct{})}
	states.inflight[runID] = call
	states.mu.Unlock()

	data, err := fetchRunnerRenderState(addrs)

	states.mu.Lock()
	call.data, call.err = data, err
	if err == nil && len(data) > 0 {
		states.cache[runID] = renderStateCacheEntry{data: data, at: time.Now()}
	}
	delete(states.inflight, runID)
	close(call.done)
	states.mu.Unlock()
	return data, err
}

func (w *Wall) handleRenderState(res http.ResponseWriter, req *http.Request) {
	runID := req.URL.Query().Get("run")
	w.mu.Lock()
	t, ok := w.tiles[runID]
	var addrs []string
	live := false
	if ok {
		addrs = append([]string(nil), t.workerAddrs...)
		live = !t.Finished && t.Status == statusRunning
	}
	w.mu.Unlock()

	if !ok || !live {
		res.WriteHeader(http.StatusNotFound)
		return
	}
	data, err := w.fetchRunnerRenderStateCached(req.Context(), runID, addrs)
	if err != nil {
		res.WriteHeader(http.StatusBadGateway)
		return
	}
	res.Header().Set("Content-Type", "application/json")
	res.Header().Set("Cache-Control", "no-store")
	res.Write(data) //nolint:errcheck // browser retries on the next render tick
}
