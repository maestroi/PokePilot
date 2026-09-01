package main

import "testing"

func TestHeartbeatTrailResetsDedupesAndCaps(t *testing.T) {
	var trail heartbeatTrail

	got := trail.add(0x0d, 5, 10)
	if len(got) != 1 || got[0] != [2]uint8{5, 10} {
		t.Fatalf("first sample = %#v", got)
	}
	got = trail.add(0x0d, 5, 10)
	if len(got) != 1 {
		t.Fatalf("duplicate sample grew trail: %#v", got)
	}

	for i := 0; i < heartbeatTrailMax+10; i++ {
		trail.add(0x0d, uint8(i+20), 11)
	}
	got = trail.add(0x0d, 99, 12)
	if len(got) != heartbeatTrailMax {
		t.Fatalf("trail length = %d, want %d", len(got), heartbeatTrailMax)
	}
	if got[len(got)-1] != [2]uint8{99, 12} {
		t.Fatalf("last sample = %#v", got[len(got)-1])
	}

	got = trail.add(0x33, 7, 8)
	if len(got) != 1 || got[0] != [2]uint8{7, 8} {
		t.Fatalf("map change did not reset trail: %#v", got)
	}

	got[0] = [2]uint8{1, 1}
	again := trail.add(0x33, 7, 8)
	if again[0] != [2]uint8{7, 8} {
		t.Fatalf("returned trail aliases internal buffer: %#v", again)
	}
}
