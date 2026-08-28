package emu

import "testing"

func TestTraceTailReturnsNewestLast(t *testing.T) {
	m := &Emu{trace: newTraceBuf()}
	for i, text := range []string{"a", "b", "c"} {
		m.trace.add(TraceEntry{Frame: uint64(i), Kind: "map", Text: text})
	}
	got := m.TraceTail(2)
	want := []string{"map: b", "map: c"}
	if len(got) != len(want) {
		t.Fatalf("TraceTail(2) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TraceTail(2)[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTraceTailShorterThanRequested(t *testing.T) {
	m := &Emu{trace: newTraceBuf()}
	m.trace.add(TraceEntry{Kind: "control", Text: "boot"})
	got := m.TraceTail(10)
	if len(got) != 1 || got[0] != "control: boot" {
		t.Fatalf("TraceTail(10) = %v, want [\"control: boot\"]", got)
	}
}
