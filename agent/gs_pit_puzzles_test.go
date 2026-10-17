package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/world"
)

func TestGSRetailStrengthBoulderAssignments(t *testing.T) {
	ice := map[int]world.Point{
		2: {X: 11, Y: 2}, 3: {X: 4, Y: 7}, 4: {X: 5, Y: 12}, 5: {X: 12, Y: 13},
	}
	gym := map[int]world.Point{
		4: {X: 8, Y: 3}, 5: {X: 2, Y: 5}, 6: {X: 8, Y: 7},
	}
	if len(gsIcePathPitTargets) != len(ice) || len(gsBlackthornPitTargets) != len(gym) {
		t.Fatal("lost an assigned retail pit")
	}
	for slot, position := range ice {
		if gsIcePathPitTargets[slot] != position {
			t.Fatalf("Ice Path slot %d bad pit %+v", slot, gsIcePathPitTargets[slot])
		}
	}
	for slot, position := range gym {
		if gsBlackthornPitTargets[slot] != position {
			t.Fatalf("Gym slot %d bad pit %+v", slot, gsBlackthornPitTargets[slot])
		}
	}
	for _, goal := range []ProgressID{
		gsprofile.ProgressIcePathBouldersDropped,
		gsprofile.ProgressBlackthornGymBouldersDropped,
	} {
		if !gsSupportedProgress(goal) {
			t.Fatalf("puzzle %s cannot run", goal)
		}
	}
}
