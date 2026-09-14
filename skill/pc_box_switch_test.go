package skill

import "testing"

func TestNextNonFullBoxChoosesFirstAvailableAfterCurrent(t *testing.T) {
	counts := [gen1BoxCount]uint8{}
	for i := range counts {
		counts[i] = gen1BoxCapacity
	}
	counts[3] = gen1BoxCapacity - 1
	counts[7] = 0

	got, ok := nextNonFullBox(counts, 1)
	if !ok || got != 3 {
		t.Fatalf("nextNonFullBox = (%d,%v), want (3,true)", got, ok)
	}
}

func TestNextNonFullBoxWrapsAround(t *testing.T) {
	counts := [gen1BoxCount]uint8{}
	for i := range counts {
		counts[i] = gen1BoxCapacity
	}
	counts[0] = 0

	got, ok := nextNonFullBox(counts, gen1BoxCount-2)
	if !ok || got != 0 {
		t.Fatalf("nextNonFullBox = (%d,%v), want (0,true)", got, ok)
	}
}

func TestNextNonFullBoxReportsAllFull(t *testing.T) {
	counts := [gen1BoxCount]uint8{}
	for i := range counts {
		counts[i] = gen1BoxCapacity
	}
	if got, ok := nextNonFullBox(counts, 4); ok || got != -1 {
		t.Fatalf("nextNonFullBox = (%d,%v), want (-1,false)", got, ok)
	}
}

func TestNextNonFullBoxNeverReturnsCurrent(t *testing.T) {
	counts := [gen1BoxCount]uint8{}
	for i := range counts {
		counts[i] = gen1BoxCapacity
	}
	counts[5] = 0
	if got, ok := nextNonFullBox(counts, 5); ok || got != -1 {
		t.Fatalf("nextNonFullBox reused current box: (%d,%v)", got, ok)
	}
}
