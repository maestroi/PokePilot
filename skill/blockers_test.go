package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

func TestPresentStationaryObjectBlockersSkipsHidden(t *testing.T) {
	h := rom.MapHeader{Objects: []rom.Object{
		{X: 2, Y: 3, Movement: rom.MovementStay},
		{X: 4, Y: 5, Movement: rom.MovementStay},
		{X: 6, Y: 7, Movement: rom.MovementWalk},
	}}
	got := presentStationaryObjectBlockers(h, map[uint8]bool{2: true})
	if !got[[2]int{2, 3}] {
		t.Fatal("visible stay object missing")
	}
	if got[[2]int{4, 5}] {
		t.Fatal("hidden stay object included")
	}
	if got[[2]int{6, 7}] {
		t.Fatal("walking object included")
	}
	if len(got) != 1 {
		t.Fatalf("blockers = %v, want only (2,3)", got)
	}
}

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
