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
	liveSpectatorFPS         = 20
	liveSpectatorFrameStride = uint64(3)

	// Flat-out execution happens in short bursts between planner calls. Keep a
	// native-time playback window, but consume it faster than 1x when backlog
	// builds so MAX runs still look fast instead of replaying old movement at
	// normal Game Boy speed.
	liveSpectatorBufferSeconds = 12
	liveSpectatorBufferSize    = liveSpectatorFPS * liveSpectatorBufferSeconds

	// Backlog thresholds are measured in sampled display frames. At >= 0.6s of
	// native game time behind, play about 4x; at >= 0.3s, play about 2x; near
	// live, return to 1x. Extreme producer bursts are compacted across the whole
	// buffered timeline rather than ending in one giant catch-up teleport.
	liveSpectatorFastBacklog = 12
	liveSpectatorMidBacklog  = 6
)

// frameSpectator is the tiny surface Emu needs from its read-only screen
// publisher. Keeping it local lets PokePilot add playback smoothing without
// changing GomeBoy's generic latest-frame spectator.
type frameSpectator interface {
	Capture(*gomeboy.Emulator) error
	// Reset starts a new visual epoch after emulator state restoration. It
	// must discard all frames captured before the restore so buffered viewers
	// can never replay an earlier boot/run before showing the restored state.
	Reset(*gomeboy.Emulator) error
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

// reset starts a new playback epoch unconditionally. Frame counters are not a
// reliable epoch signal: a resumed checkpoint can have a larger frame number
// than the worker boot frames that were captured before the lease started.
// In that case rollback detection in push cannot distinguish the two runs.
func (q *liveFrameQueue) reset(frame uint64, png []byte) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.frames = q.frames[:0]
	q.last = liveFrame{}
	q.haveLast = false
	q.newest = 0
	q.haveNew = false

	if len(png) == 0 {
		return
	}
	next := liveFrame{frame: frame, png: png}
	q.frames = append(q.frames, next)
	q.newest = frame
	q.haveNew = true
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

	next := liveFrame{frame: frame, png: png}
	q.newest = frame
	q.haveNew = true

	// When MAX speed fills the window, compact the whole buffered timeline by
	// keeping one representative from each adjacent pair. That preserves a
	// continuous fast-forward through the entire burst. The previous design
	// kept the first 12 seconds at 1x plus one newest pending frame, which is
	// exactly the "slow for a while, then teleport" failure mode.
	if len(q.frames) >= q.capacity {
		q.compactLocked()
	}
	q.frames = append(q.frames, next)
}

func (q *liveFrameQueue) compactLocked() {
	n := len(q.frames)
	if n <= 1 {
		q.frames = q.frames[:0]
		return
	}

	// Pick the newer frame from each adjacent pair while ensuring the newest
	// buffered frame survives both odd and even lengths.
	start := (n + 1) % 2
	write := 0
	for read := start; read < n; read += 2 {
		q.frames[write] = q.frames[read]
		write++
	}
	q.frames = q.frames[:write]
}

func playbackAdvance(depth int) int {
	switch {
	case depth >= liveSpectatorFastBacklog:
		return 4
	case depth >= liveSpectatorMidBacklog:
		return 2
	default:
		return 1
	}
}

func (q *liveFrameQueue) nextPlayback() (liveFrame, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.frames) > 0 {
		advance := playbackAdvance(len(q.frames))
		if advance > len(q.frames) {
			advance = len(q.frames)
		}
		frame := q.frames[advance-1]
		copy(q.frames, q.frames[advance:])
		q.frames = q.frames[:len(q.frames)-advance]
		q.last = frame
		q.haveLast = true
		return frame, true
	}
	if q.haveLast {
		return q.last, true
	}
	return liveFrame{}, false
}

func (q *liveFrameQueue) latest() (liveFrame, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if n := len(q.frames); n > 0 {
		return q.frames[n-1], true
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

// Reset is called after a successful emulator LoadState. Clear playback first
// even if PNG encoding fails: keeping an old frame would make a resumed run
// look like it restarted and then teleported. On success, seed the new epoch
// immediately with the restored screen so the next browser poll starts at the
// checkpoint rather than waiting for another stepped frame.
func (s *liveSpectator) Reset(e *gomeboy.Emulator) error {
	frame := e.FrameCount()
	s.queue.reset(0, nil)
	s.haveCapture = false

	png, err := e.PNG()
	if err != nil {
		return err
	}
	s.queue.reset(frame, png)
	s.lastCapture = frame
	s.haveCapture = true
	return nil
}

// Capture samples emulator time rather than worker wall time. Flat-out
// execution can produce hundreds of emulator frames between browser reads;
// one native-time display frame every ~3 emulator frames enters the bounded
// queue. The HTTP consumer adaptively fast-forwards that queue when it falls
// behind, so execution stays fast without presenting a 1x backlog slideshow.
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

		var (
			frame liveFrame
			ok    bool
		)
		if r.URL.Query().Get("buffered") == "1" {
			frame, ok = s.queue.nextPlayback()
		} else {
			// Keep the original /frame.png contract for finish snapshots and
			// other point reads: callers that do not opt into playback always
			// get the newest image, never a queued historical frame.
			frame, ok = s.queue.latest()
		}
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
