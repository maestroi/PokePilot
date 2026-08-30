package emu

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestTraceBufRingDropsOldest(t *testing.T) {
	buf := newTraceBuf()
	for i := 0; i < traceCapacity+10; i++ {
		buf.add(TraceEntry{Frame: uint64(i), Kind: "map", Text: "x"})
	}
	got := buf.snapshot()
	if len(got) != traceCapacity {
		t.Fatalf("len = %d, want %d", len(got), traceCapacity)
	}
	if got[0].Frame != 10 {
		t.Fatalf("oldest kept entry has Frame %d, want 10 (first %d dropped)", got[0].Frame, 10)
	}
	if last := got[len(got)-1]; last.Frame != uint64(traceCapacity+9) {
		t.Fatalf("newest entry has Frame %d, want %d", last.Frame, traceCapacity+9)
	}
}

func TestTraceBufSnapshotIsACopy(t *testing.T) {
	buf := newTraceBuf()
	buf.add(TraceEntry{Frame: 1, Kind: "map", Text: "a"})

	first := buf.snapshot()
	first[0].Text = "mutated"

	second := buf.snapshot()
	if second[0].Text != "a" {
		t.Fatalf("mutating a returned snapshot affected a later read: got %q", second[0].Text)
	}
}

func TestTraceBufConcurrentAppendAndRead(t *testing.T) {
	buf := newTraceBuf()
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			buf.add(TraceEntry{Frame: uint64(i), Kind: "map", Text: "x"})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = buf.snapshot()
		}
	}()
	wg.Wait()
}

func TestDiffChangedNoEmitWhenUnchanged(t *testing.T) {
	prev := uint8(0x26)
	if _, changed := diffChanged(&prev, uint8(0x26)); changed {
		t.Fatal("diffChanged reported a change for an equal value")
	}
	if prev != 0x26 {
		t.Fatalf("prev mutated on no-op diff: got %#x", prev)
	}
}

func TestDiffChangedEmitsOneOnChange(t *testing.T) {
	prev := uint8(0x26)
	old, changed := diffChanged(&prev, uint8(0x25))
	if !changed {
		t.Fatal("diffChanged did not report a change")
	}
	if old != 0x26 {
		t.Fatalf("old = %#x, want %#x", old, 0x26)
	}
	if prev != 0x25 {
		t.Fatalf("prev = %#x, want %#x", prev, 0x25)
	}

	// A second call with the same value now emits nothing.
	if _, changed := diffChanged(&prev, uint8(0x25)); changed {
		t.Fatal("diffChanged reported a change on a repeated value")
	}
}

func TestAddAssignsMonotonicSeq(t *testing.T) {
	b := newTraceBuf()
	for i := 0; i < 5; i++ {
		b.add(TraceEntry{Kind: "map"})
	}
	got := b.snapshot()
	for i, e := range got {
		if want := uint64(i + 1); e.Seq != want {
			t.Fatalf("entry %d Seq = %d, want %d", i, e.Seq, want)
		}
	}
}

// Once the ring wraps, length stops growing but Seq must keep climbing —
// that is the whole reason consumers page on Seq instead of an index.
func TestSeqKeepsClimbingAfterWrap(t *testing.T) {
	b := newTraceBuf()
	for i := 0; i < traceCapacity+10; i++ {
		b.add(TraceEntry{Kind: "map"})
	}
	got := b.snapshot()
	if len(got) != traceCapacity {
		t.Fatalf("len = %d, want %d", len(got), traceCapacity)
	}
	if first, last := got[0].Seq, got[len(got)-1].Seq; first != 11 || last != uint64(traceCapacity+10) {
		t.Fatalf("Seq range = %d..%d, want 11..%d", first, last, traceCapacity+10)
	}
}

func TestEachBufHasItsOwnRun(t *testing.T) {
	a, b := newTraceBuf(), newTraceBuf()
	if a.run == "" {
		t.Fatal("run is empty")
	}
	if a.run == b.run {
		t.Fatalf("two buffers share run %q", a.run)
	}
}

// TestTraceStatsRoundTrip: the stats blob is carried verbatim and is absent
// until something sets it, so a page polling a run with no planner tally
// sees no panel rather than an empty one.
func TestTraceStatsRoundTrip(t *testing.T) {
	b := newTraceBuf()
	rec := httptest.NewRecorder()
	b.serveHTTP(rec, httptest.NewRequest(http.MethodGet, "/trace.json", nil))
	if strings.Contains(rec.Body.String(), "stats") {
		t.Fatalf("unset stats must be omitted, got %s", rec.Body.String())
	}

	b.stats = json.RawMessage(`{"repeats":3}`)
	rec = httptest.NewRecorder()
	b.serveHTTP(rec, httptest.NewRequest(http.MethodGet, "/trace.json", nil))
	var got tracePayload
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Stats) != `{"repeats":3}` {
		t.Fatalf("stats = %s, want the blob verbatim", got.Stats)
	}
}
