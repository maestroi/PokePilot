package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

func TestSelectedBoulderMovablesRestrictsSlots(t *testing.T) {
	in := []world.Movable{
		{ID: 1, Pos: world.Point{X: 5, Y: 14}},
		{ID: 2, Pos: world.Point{X: 3, Y: 15}},
		{ID: 3, Pos: world.Point{X: 8, Y: 14}},
		{ID: 4, Pos: world.Point{X: 9, Y: 14}},
	}
	got := selectedBoulderMovables(in, map[int]bool{2: true})
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("selected movables = %v, want only slot 2", got)
	}
	if all := selectedBoulderMovables(in, nil); len(all) != len(in) {
		t.Fatalf("nil selection returned %d movables, want %d", len(all), len(in))
	}
}

func TestSeafoamDropStagesMatchROMCoordinates(t *testing.T) {
	cases := []struct {
		mapID uint8
		stage state.SeafoamDropStage
		holes [2]world.Point
	}{
		{seafoam1FMap, state.SeafoamDrop1F, [2]world.Point{{X: 17, Y: 6}, {X: 24, Y: 6}}},
		{seafoamB1FMap, state.SeafoamDropB1F, [2]world.Point{{X: 18, Y: 6}, {X: 23, Y: 6}}},
		{seafoamB2FMap, state.SeafoamDropB2F, [2]world.Point{{X: 19, Y: 6}, {X: 22, Y: 6}}},
		{seafoamB3FMap, state.SeafoamDropB3F, [2]world.Point{{X: 3, Y: 16}, {X: 6, Y: 16}}},
	}
	for _, tc := range cases {
		spec, ok := seafoamDropStageForMap(tc.mapID)
		if !ok {
			t.Fatalf("map %#02x has no Seafoam stage", tc.mapID)
		}
		if spec.Stage != tc.stage || spec.Holes != tc.holes {
			t.Fatalf("map %#02x stage = %+v, want stage=%d holes=%v", tc.mapID, spec, tc.stage, tc.holes)
		}
		if spec.Slots != [2]int{1, 2} {
			t.Fatalf("map %#02x source slots = %v, want [1 2]", tc.mapID, spec.Slots)
		}
	}
	if _, ok := seafoamDropStageForMap(seafoamB4FMap); ok {
		t.Fatal("B4F unexpectedly treated as a boulder-drop source floor")
	}
}

func TestSeafoamBoulderDropSpecBindsSourceSlotAndEvent(t *testing.T) {
	stage, ok := seafoamDropStageForMap(seafoamB3FMap)
	if !ok {
		t.Fatal("missing B3F Seafoam stage")
	}
	firstEvent, secondEvent, ok := state.SeafoamDropEvents(state.SeafoamDropB3F)
	if !ok {
		t.Fatal("missing B3F Seafoam events")
	}
	for index, wantEvent := range []state.Event{firstEvent, secondEvent} {
		spec, err := seafoamBoulderDropSpec(stage, index)
		if err != nil {
			t.Fatalf("drop %d: %v", index, err)
		}
		hole := stage.Holes[index]
		if len(spec.Targets) != 1 || spec.Targets[0] != hole {
			t.Fatalf("drop %d target = %v, want %v", index, spec.Targets, hole)
		}
		if !spec.MovableIDs[stage.Slots[index]] || len(spec.MovableIDs) != 1 {
			t.Fatalf("drop %d movable slots = %v, want only %d", index, spec.MovableIDs, stage.Slots[index])
		}
		if !spec.TerminalTargets[[2]int{hole.X, hole.Y}] {
			t.Fatalf("drop %d hole %v is not terminal", index, hole)
		}
		if !spec.HasCompleteEvent || spec.CompleteEvent != wantEvent {
			t.Fatalf("drop %d completion = (%v,%#x), want (true,%#x)", index, spec.HasCompleteEvent, spec.CompleteEvent, wantEvent)
		}
	}
}
