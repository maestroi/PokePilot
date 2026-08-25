package emu

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/maestroi/pokepilot/red/sym"
)

// TraceEntry is one line of the derived event trace served at /trace.json.
// The trace is derived entirely from RAM state changes observed by
// sampleTrace; nothing in skill/ is instrumented.
type TraceEntry struct {
	Frame uint64 `json:"frame"`
	Kind  string `json:"kind"` // "map", "dialogue", "control", "battle"
	Text  string `json:"text"`
}

const traceCapacity = 256

// traceBuf is a fixed-capacity ring buffer of TraceEntry, safe for
// concurrent use: entries are appended from the goroutine stepping the
// emulator and read from the HTTP handler goroutine.
type traceBuf struct {
	mu      sync.Mutex
	entries []TraceEntry
}

func newTraceBuf() *traceBuf {
	return &traceBuf{entries: make([]TraceEntry, 0, traceCapacity)}
}

// add appends e, dropping the oldest entry once the buffer is full.
func (t *traceBuf) add(e TraceEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = append(t.entries, e)
	if len(t.entries) > traceCapacity {
		t.entries = t.entries[len(t.entries)-traceCapacity:]
	}
}

// snapshot returns a copy of the current entries, newest last. The caller
// may freely mutate the result.
func (t *traceBuf) snapshot() []TraceEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TraceEntry, len(t.entries))
	copy(out, t.entries)
	return out
}

func (t *traceBuf) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(t.snapshot())
}

// diffChanged is the change-detection primitive: it reports whether cur
// differs from *prev and, if so, updates *prev and returns the old value.
// Pure and generic so it can be unit tested without an emulator.
func diffChanged[T comparable](prev *T, cur T) (old T, changed bool) {
	old = *prev
	if cur == old {
		return old, false
	}
	*prev = cur
	return old, true
}

// traceState holds the previously-sampled values used for change
// detection. primed is false until the first sample, so process start
// never looks like a spurious transition from the zero value.
type traceState struct {
	primed    bool
	mapID     uint8
	joyIgnore uint8
	inBattle  bool
	dialogue  string // last text emitted to the trace

	// The text box types out one character per few frames, so a raw read
	// changes almost every sample while typing. pending/pendingStable
	// debounce that: only a reading seen on two consecutive samples (i.e.
	// typing has caught up, or paused waiting for input) is considered
	// settled and worth emitting.
	pending       string
	pendingStable bool
}

// sampleTrace reads the current RAM state and appends trace entries for
// anything that changed since the last sample. It runs on the same
// goroutine and at the same cadence as capture(), so nothing races with
// emulation.
func (m *Emu) sampleTrace() {
	if m.trace == nil {
		return
	}
	frame := m.e.FrameCount()
	st := &m.traceSt

	mapID := m.Peek8(sym.CurMap)
	joyIgnore := m.Peek8(sym.JoyIgnore)
	inBattle := m.Peek8(sym.IsInBattle) != 0
	if !st.primed {
		st.mapID, st.joyIgnore, st.inBattle = mapID, joyIgnore, inBattle
		st.primed = true
		return
	}

	if old, ok := diffChanged(&st.mapID, mapID); ok {
		m.trace.add(TraceEntry{Frame: frame, Kind: "map", Text: fmt.Sprintf("map %#02x -> %#02x", old, mapID)})
	}

	if wasIgnoring, isIgnoring := st.joyIgnore != 0, joyIgnore != 0; wasIgnoring != isIgnoring {
		st.joyIgnore = joyIgnore
		if isIgnoring {
			m.trace.add(TraceEntry{Frame: frame, Kind: "control", Text: fmt.Sprintf("control lost (joyIgnore %#02x)", joyIgnore)})
		} else {
			m.trace.add(TraceEntry{Frame: frame, Kind: "control", Text: "control regained"})
		}
	} else {
		st.joyIgnore = joyIgnore
	}

	if m.onSample != nil {
		m.onSample(m)
	}

	if _, ok := diffChanged(&st.inBattle, inBattle); ok {
		if inBattle {
			m.trace.add(TraceEntry{Frame: frame, Kind: "battle", Text: "battle started"})
		} else {
			m.trace.add(TraceEntry{Frame: frame, Kind: "battle", Text: "battle ended"})
		}
	}
}

// TraceNote appends an entry to the trace from outside emu.
//
// emu deliberately knows nothing about Pokemon: it samples only the handful
// of addresses in red/sym that describe the machine's situation (map, input
// gating, battle). Anything that needs decoding — dialogue text, party state,
// a planner's decision — is pushed in from a layer that is allowed to
// understand it. Safe to call from the goroutine stepping the emulator.
func (m *Emu) TraceNote(kind, text string) {
	if m.trace == nil {
		return
	}
	m.trace.add(TraceEntry{Frame: m.e.FrameCount(), Kind: kind, Text: text})
}

// OnSample registers fn to run on every trace sample, on the goroutine that
// steps the emulator, so it may read emulator memory without racing. Use it
// to record anything emu cannot decode by itself. Passing nil clears it.
func (m *Emu) OnSample(fn func(*Emu)) { m.onSample = fn }
