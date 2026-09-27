package main

import (
	"testing"
	"time"
)

func TestFrameStallWatchDumpsOncePerStall(t *testing.T) {
	var w frameStallWatch
	t0 := time.Unix(0, 0)
	steps := []struct {
		frame uint64
		at    time.Duration
		want  bool
	}{
		{100, 0, false},
		{100, frameStallDumpAfter - time.Second, false},
		{100, frameStallDumpAfter, true},
		{100, 2 * frameStallDumpAfter, false}, // same stall: no second dump
		{101, 2*frameStallDumpAfter + time.Second, false},
		{101, 3*frameStallDumpAfter + time.Second, true}, // new stall re-armed
	}
	for i, s := range steps {
		if got := w.observe(s.frame, t0.Add(s.at)); got != s.want {
			t.Fatalf("step %d: observe(%d, %s) = %v, want %v", i, s.frame, s.at, got, s.want)
		}
	}
}
