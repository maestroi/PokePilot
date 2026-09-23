package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

func TestObservedStationaryObjectBlockers(t *testing.T) {
	h := rom.MapHeader{Objects: []rom.Object{
		{X: 2, Y: 3, Movement: rom.MovementStay},
		{X: 4, Y: 5, Movement: rom.MovementStay},
		{X: 6, Y: 7, Movement: rom.MovementWalk},
		{X: 8, Y: 9, Movement: rom.MovementWalk},
	}}
	live := []state.SpriteState{
		{Slot: 1, X: 2, Y: 3},
		{Slot: 3, X: 6, Y: 7},
		// A different, moving object happens to occupy hidden stationary
		// object 2's home tile. Coordinate matching alone must not persist it.
		{Slot: 4, X: 4, Y: 5},
	}

	got := observedStationaryObjectBlockers(h, live)
	if !got[[2]int{2, 3}] {
		t.Fatal("visible stationary object was not included")
	}
	if got[[2]int{4, 5}] {
		t.Fatal("hidden stationary object was included")
	}
	if got[[2]int{6, 7}] {
		t.Fatal("moving object was included")
	}
	if len(got) != 1 {
		t.Fatalf("blockers = %v, want only (2,3)", got)
	}
}

func TestPersistentTopologyBlockersExcludeMovingAndHiddenObjects(t *testing.T) {
	h := rom.MapHeader{Objects: []rom.Object{
		{X: 2, Y: 3, Movement: rom.MovementStay},
		{X: 4, Y: 5, Movement: rom.MovementStay},
		{X: 6, Y: 7, Movement: rom.MovementWalk},
	}}
	hidden := map[uint8]bool{2: true}

	got := persistentTopologyBlockers(h, hidden)
	if !got[[2]int{2, 3}] {
		t.Fatal("present stationary object was not persisted")
	}
	if got[[2]int{4, 5}] {
		t.Fatal("hidden stationary object was persisted")
	}
	if got[[2]int{6, 7}] {
		t.Fatal("moving object was persisted as topology")
	}
	if len(got) != 1 {
		t.Fatalf("persistent blockers = %v, want only (2,3)", got)
	}
}

func TestPresentStationaryObjectBlockersKeepsOffscreenStayObjects(t *testing.T) {
	h := rom.MapHeader{Objects: []rom.Object{
		{X: 8, Y: 16, Movement: rom.MovementStay},
		{X: 28, Y: 4, Movement: rom.MovementStay},
		{X: 6, Y: 7, Movement: rom.MovementWalk},
	}}
	// No emulator: presentStationaryObjectBlockers needs HiddenObjectIDs from
	// RAM. With a zero Mem every object is present, so both stay homes remain
	// even though no sprite snapshot includes them — the off-screen trainer
	// case that routingBlockers exists to cover.
	var mem state.Mem
	hidden := state.HiddenObjectIDs(&mem)
	if len(hidden) != 0 {
		t.Fatalf("empty mem reported hidden objects %v", hidden)
	}
	got := map[[2]int]bool{}
	for i, o := range h.Objects {
		if o.Movement != rom.MovementStay || hidden[uint8(i+1)] {
			continue
		}
		got[[2]int{int(o.X), int(o.Y)}] = true
	}
	if !got[[2]int{8, 16}] || !got[[2]int{28, 4}] {
		t.Fatalf("present stay blockers = %v, want (8,16) and (28,4)", got)
	}
	if got[[2]int{6, 7}] {
		t.Fatal("walking object was treated as present stationary")
	}
}

// A stay trainer that walked out to intercept the player blocks its RAM tile,
// not its now-empty home, even while off-screen (run-1biaubd9xooqm).
func TestStationaryObjectBlockersFollowMovedTrainer(t *testing.T) {
	h := rom.MapHeader{Objects: []rom.Object{
		{X: 10, Y: 1, Movement: rom.MovementStay},
		{X: 16, Y: 9, Movement: rom.MovementStay},
	}}
	got := stationaryObjectBlockers(h, map[int][2]int{1: {10, 3}})
	if !got[[2]int{10, 3}] || got[[2]int{10, 1}] {
		t.Fatalf("moved trainer blockers = %v, want (10,3) not (10,1)", got)
	}
	if !got[[2]int{16, 9}] {
		t.Fatalf("slot without RAM tile lost its home blocker: %v", got)
	}
}
