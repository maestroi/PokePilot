package state

import "testing"

func TestSeafoamDropEventsMatchDecomp(t *testing.T) {
	events := parseEventConstants(t)
	cases := []struct {
		stage  SeafoamDropStage
		first  string
		second string
	}{
		{SeafoamDrop1F, "EVENT_SEAFOAM1_BOULDER1_DOWN_HOLE", "EVENT_SEAFOAM1_BOULDER2_DOWN_HOLE"},
		{SeafoamDropB1F, "EVENT_SEAFOAM2_BOULDER1_DOWN_HOLE", "EVENT_SEAFOAM2_BOULDER2_DOWN_HOLE"},
		{SeafoamDropB2F, "EVENT_SEAFOAM3_BOULDER1_DOWN_HOLE", "EVENT_SEAFOAM3_BOULDER2_DOWN_HOLE"},
		{SeafoamDropB3F, "EVENT_SEAFOAM4_BOULDER1_DOWN_HOLE", "EVENT_SEAFOAM4_BOULDER2_DOWN_HOLE"},
	}
	for _, tc := range cases {
		first, second, ok := SeafoamDropEvents(tc.stage)
		if !ok {
			t.Fatalf("stage %d has no event mapping", tc.stage)
		}
		if got, want := uint16(first), events[tc.first]; got != want {
			t.Fatalf("%s = %#x, want %#x", tc.first, got, want)
		}
		if got, want := uint16(second), events[tc.second]; got != want {
			t.Fatalf("%s = %#x, want %#x", tc.second, got, want)
		}
	}
}

func TestSeafoamCurrentsStoppedNeedsBothFinalDrops(t *testing.T) {
	var mem Mem
	first, second, ok := SeafoamDropEvents(SeafoamDropB3F)
	if !ok {
		t.Fatal("missing B3F Seafoam drop events")
	}
	if SeafoamCurrentsStopped(&mem) {
		t.Fatal("fresh state reported Seafoam currents stopped")
	}
	setTestEvent(&mem, first)
	if SeafoamCurrentsStopped(&mem) {
		t.Fatal("one final boulder incorrectly stopped Seafoam currents")
	}
	setTestEvent(&mem, second)
	if !SeafoamCurrentsStopped(&mem) {
		t.Fatal("both final boulder drops did not stop Seafoam currents")
	}
}
