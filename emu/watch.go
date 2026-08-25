package emu

import (
	"net"
	"net/http"
	"time"

	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
)

// Watch serves the emulator's screen over HTTP at addr so a human can see
// what the agent is doing. It returns the address actually listened on,
// which is useful when addr uses port 0.
//
// Frames are captured on the goroutine that steps the emulator, once every
// everyFrames frames, so nothing races with emulation and the caller pays a
// predictable encoding cost. everyFrames <= 0 disables capture.
//
// This is a debugging and demonstration surface only. Nothing in PokePilot
// may read gameplay truth from it — see docs/DESIGN.md.
func (m *Emu) Watch(addr string, everyFrames int) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	m.spec = gomeboy.NewSpectator()
	m.specEvery = everyFrames
	go http.Serve(ln, m.spec.Handler()) //nolint:errcheck // serves until the process exits
	return ln.Addr().String(), nil
}

// capture refreshes what Watch serves, if watching is on and enough frames
// have passed. Errors are ignored: a dropped preview frame must never affect
// emulation.
func (m *Emu) capture() {
	if m.spec == nil || m.specEvery <= 0 {
		return
	}
	if m.e.FrameCount()-m.lastCapture < uint64(m.specEvery) {
		return
	}
	m.lastCapture = m.e.FrameCount()
	_ = m.spec.Capture(m.e)
}

// Pace throttles emulation to about fps frames per second, so a human can
// follow what the agent is doing. Zero or negative runs as fast as the CPU
// allows, which is the default and what tests use.
//
// Pacing is wall-clock only. It cannot change what the game does, because
// every skill waits on RAM predicates rather than on elapsed time.
func (m *Emu) Pace(fps int) {
	if fps <= 0 {
		m.frameDur = 0
		return
	}
	m.frameDur = time.Second / time.Duration(fps)
	m.nextFrame = time.Time{}
}

// throttle sleeps until n frames' worth of wall clock has passed.
func (m *Emu) throttle(n int) {
	if m.frameDur <= 0 {
		return
	}
	if m.nextFrame.IsZero() {
		m.nextFrame = time.Now()
	}
	m.nextFrame = m.nextFrame.Add(time.Duration(n) * m.frameDur)
	d := time.Until(m.nextFrame)
	switch {
	case d > 0:
		time.Sleep(d)
	case d < -time.Second:
		// ponytail: fell more than a second behind (a slow capture, a
		// descheduled process). Resync instead of sprinting to catch up,
		// which would look worse than the hitch we already took.
		m.nextFrame = time.Now()
	}
}
