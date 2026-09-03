package emu

import (
	"crypto/rand"
	"encoding/hex"
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
	// Seq is monotonic for the life of the process. Consumers must page on
	// it, never on a slice index: the buffer is a ring, so once it wraps the
	// length stops changing while entries keep arriving.
	Seq   uint64 `json:"seq"`
	Frame uint64 `json:"frame"`
	Kind  string `json:"kind"` // "map", "dialogue", "control", "battle", "llm", "round"
	Text  string `json:"text"`
}

const traceCapacity = 256

// traceBuf is a fixed-capacity ring buffer of TraceEntry, safe for
// concurrent use: entries are appended from the goroutine stepping the
// emulator and read from the HTTP handler goroutine.
type traceBuf struct {
	mu      sync.Mutex
	entries []TraceEntry
	nextSeq uint64

	// header is a one-line description of the run, pinned above the trace
	// so it survives scrolling. emu does not compose it: the layer that
	// knows what this run is sets it.
	header string

	// stats is an opaque JSON blob the watching layer sets: emu carries it
	// to the page without interpreting a byte of it, exactly as it carries
	// header. What counts as a run statistic is a question about Pokemon
	// and about planners, and emu is not allowed to know either.
	stats json.RawMessage

	// player is an opaque JSON blob the watching layer sets for the live
	// trainer snapshot, carried the same way as stats.
	player json.RawMessage

	// run identifies this process's trace. A consumer that sees a different
	// run must discard what it has: sequence numbers restart from scratch,
	// so without this a reconnecting page replays the whole trace again.
	run string
}

func newTraceBuf() *traceBuf {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return &traceBuf{
		entries: make([]TraceEntry, 0, traceCapacity),
		run:     hex.EncodeToString(b[:]),
	}
}

// add appends e, dropping the oldest entry once the buffer is full.
func (t *traceBuf) add(e TraceEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextSeq++
	e.Seq = t.nextSeq
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

// tracePayload is what /trace.json returns. Entries are wrapped rather than
// sent bare so a consumer can tell one process's trace from another's.
type tracePayload struct {
	Run     string          `json:"run"`
	Header  string          `json:"header"`
	Stats   json.RawMessage `json:"stats,omitempty"`
	Player  json.RawMessage `json:"player,omitempty"`
	Entries []TraceEntry    `json:"entries"`
}

func (t *traceBuf) serveHTTP(w http.ResponseWriter, r *http.Request) {
	t.mu.Lock()
	run, header, stats, player := t.run, t.header, t.stats, t.player
	t.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tracePayload{Run: run, Header: header, Stats: stats, Player: player, Entries: t.snapshot()})
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

// TraceHeader pins a one-line description of this run above the trace
// panel — what is driving it, and the seed, which is the only thing that
// makes one run differ from another. Safe to call at any time.
func (m *Emu) TraceHeader(text string) {
	if m.trace == nil {
		return
	}
	m.trace.mu.Lock()
	m.trace.header = text
	m.trace.mu.Unlock()
}

// TraceStats replaces the statistics blob served alongside the trace. v is
// marshalled here and carried verbatim; a value that will not marshal is
// dropped, because a broken statistic must never take down a run. Safe to
// call at any time, from any goroutine.
func (m *Emu) TraceStats(v any) {
	if m.trace == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	m.trace.mu.Lock()
	m.trace.stats = b
	m.trace.mu.Unlock()
}

// TracePlayer replaces the player-snapshot blob served alongside the
// trace. v is marshalled here and carried verbatim; a value that will
// not marshal is dropped. Safe to call at any time, from any goroutine.
func (m *Emu) TracePlayer(v any) {
	if m.trace == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	m.trace.mu.Lock()
	m.trace.player = b
	m.trace.mu.Unlock()
}

// OnSample registers fn to run on every trace sample, on the goroutine that
// steps the emulator, so it may read emulator memory without racing. Use it
// to record anything emu cannot decode by itself. Passing nil clears it.
// A second OnSample call replaces the first: each farm lease starts clean.
func (m *Emu) OnSample(fn func(*Emu)) { m.onSample = fn }

// AlsoSample appends fn after the current OnSample hook. agent.Run's
// dialogue tape must not wipe a farm heartbeat (or the local watch
// TracePlayer) that was already installed — that replacement froze the
// console on Oak's Lab, the last tile sampled before the tape took over.
func (m *Emu) AlsoSample(fn func(*Emu)) {
	if fn == nil {
		return
	}
	prev := m.onSample
	if prev == nil {
		m.onSample = fn
		return
	}
	m.onSample = func(e *Emu) {
		prev(e)
		fn(e)
	}
}

// TraceTail returns the last n trace lines as "kind: text", newest last.
// Used to fill the trace_tail field of a farm.FinishReport; nothing in
// emu imports farm, this just exposes data already collected.
func (m *Emu) TraceTail(n int) []string {
	entries := m.trace.snapshot()
	if n > len(entries) {
		n = len(entries)
	}
	entries = entries[len(entries)-n:]
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Kind + ": " + e.Text
	}
	return out
}
