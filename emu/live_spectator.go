package emu

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

const (
	// Pokemon Red advances at just under 60 emulator frames per second. The
	// public/operator UIs cap their live frame fetches at 20 fps, so retaining
	// one image every three emulator frames makes that 20 fps clock represent
	// native game time instead of whatever wall-clock speed the worker happens
	// to execute at.
	liveSpectatorFrameStride = uint64(3)

	// Keep at most one second of display frames. When a flat-out worker gets
	// farther ahead, dropping the oldest buffered images keeps spectators near
	// live instead of letting latency grow without bound.
	liveSpectatorBufferSize = 20
)

// frameSpectator is the tiny surface Emu needs from its read-only screen
// publisher. Keeping it local lets PokePilot add playback smoothing without
// changing GomeBoy's generic latest-frame spectator.
type frameSpectator interface {
	Capture(*gomeboy.Emulator) error
	Handler() http.Handler
}

type liveFrame struct {
	frame uint64
	png   []byte
}

type liveFrameQueue struct {
	mu sync.Mutex

	capacity int
	frames   []liveFrame
	last     liveFrame
	haveLast bool
	newest   uint64
	haveNew  bool
}

func newLiveFrameQueue(capacity int) *liveFrameQueue {
	if capacity < 1 {
		capacity = 1
	}
	return &liveFrameQueue{capacity: capacity}
}

func (q *liveFrameQueue) push(frame uint64, png []byte) {
	if len(png) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	// Farm workers restore an earlier checked state between leases. A lower
	// frame number therefore means a new playback epoch; old queued images must
	// not leak into the new run.
	if q.haveNew && frame < q.newest {
		q.frames = q.frames[:0]
		q.last = liveFrame{}
		q.haveLast = false
		q.haveNew = false
	}
	if q.haveNew && frame == q.newest {
		return
	}

	q.frames = append(q.frames, liveFrame{frame: frame, png: png})
	q.newest = frame
	q.haveNew = true
	if len(q.frames) > q.capacity {
		drop := len(q.frames) - q.capacity
		copy(q.frames, q.frames[drop:])
		q.frames = q.frames[:q.capacity]
	}
}

func (q *liveFrameQueue) next() (liveFrame, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.frames) > 0 {
		frame := q.frames[0]
		copy(q.frames, q.frames[1:])
		q.frames = q.frames[:len(q.frames)-1]
		q.last = frame
		q.haveLast = true
		return frame, true
	}
	if q.haveLast {
		return q.last, true
	}
	return liveFrame{}, false
}

type liveSpectator struct {
	queue *liveFrameQueue

	captureEvery uint64
	lastCapture  uint64
	haveCapture  bool
}

func newLiveSpectator(captureEvery int) *liveSpectator {
	stride := liveSpectatorFrameStride
	if captureEvery > int(stride) {
		stride = uint64(captureEvery)
	}
	return &liveSpectator{
		queue:        newLiveFrameQueue(liveSpectatorBufferSize),
		captureEvery: stride,
	}
}

// Capture samples emulator time rather than worker wall time. Flat-out
// execution can produce hundreds of emulator frames between browser reads;
// only one native-time display frame every ~3 emulator frames enters the
// bounded queue, and the HTTP consumer drains that queue at its steady 20 fps.
func (s *liveSpectator) Capture(e *gomeboy.Emulator) error {
	frame := e.FrameCount()
	if s.haveCapture {
		if frame < s.lastCapture {
			// A restored farm checkpoint starts a new frame epoch.
			s.haveCapture = false
		} else if frame-s.lastCapture < s.captureEvery {
			return nil
		}
	}

	png, err := e.PNG()
	if err != nil {
		return err
	}
	s.queue.push(frame, png)
	s.lastCapture = frame
	s.haveCapture = true
	return nil
}

func (s *liveSpectator) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/frame.png" {
			http.NotFound(w, r)
			return
		}
		frame, ok := s.queue.next()
		if !ok {
			http.Error(w, "no frame captured yet", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-PokePilot-Frame", strconv.FormatUint(frame.frame, 10))
		_, _ = w.Write(frame.png)
	})
}
