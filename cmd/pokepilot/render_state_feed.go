package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/emu"
	redrenderstate "github.com/maestroi/pokepilot/red/renderstate"
	protocol "github.com/maestroi/pokepilot/renderstate"
)

// renderStateFeed is the watch-server side of the semantic renderer. Capture
// runs only on the emulator stepping goroutine; HTTP readers only copy the
// already-encoded payload, so public spectators can never race gameplay RAM.
type renderStateFeed struct {
	mu        sync.RWMutex
	payload   []byte
	lastFrame uint64
	haveFrame bool
}

func newRenderStateFeed() *renderStateFeed { return &renderStateFeed{} }

func (f *renderStateFeed) reset() {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.payload = nil
	f.lastFrame = 0
	f.haveFrame = false
	f.mu.Unlock()
}

func (f *renderStateFeed) capture(m *emu.Emu, producer *redrenderstate.Producer) {
	if f == nil || m == nil || producer == nil {
		return
	}
	frame := m.FrameCount()

	f.mu.RLock()
	duplicate := f.haveFrame && f.lastFrame == frame
	f.mu.RUnlock()
	if duplicate {
		return
	}

	state, err := producer.Snapshot(m, protocol.FrameMeta{
		Frame:            frame,
		CapturedAtUnixMS: time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return
	}

	f.mu.Lock()
	f.payload = append(f.payload[:0], payload...)
	f.lastFrame = frame
	f.haveFrame = true
	f.mu.Unlock()
}

func (f *renderStateFeed) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	f.mu.RLock()
	payload := append([]byte(nil), f.payload...)
	f.mu.RUnlock()
	if len(payload) == 0 {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "semantic render state unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(payload) //nolint:errcheck // browser retries the read
}
