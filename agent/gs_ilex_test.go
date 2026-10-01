package agent

import "testing"

// gsIlexBirdBounceFacings is the branch table from maps/IlexForest.asm in
// pret/pokegold: a position whose `FarfetchdCryAndCheckFacing` compares the
// player's facing to one of these sends the bird backwards instead of on to the
// next position. Position 1 has no facing check, and position 10 only prints
// its text, so neither can bounce.
//
// The player's facing is read out of wPlayerDirection by `readvar VAR_FACING`
// (engine/overworld/variables.asm: .PlayerFacing), and the direction constants
// are DOWN=0, UP=1, LEFT=2, RIGHT=3 (constants/ram_constants.asm).
var gsIlexBirdBounceFacings = map[int][]string{
	2: {"down"}, // -> position 8
	3: {"left"}, // -> position 2
	4: {"up"},   // -> position 3
	5: {"up", "left", "right"},
	6: {"right"},
	7: {"down", "left"},
	8: {"up", "left", "right"},
	9: {"down", "right"},
}

// TestGSIlexBirdStepsFaceForwardBranch pins the staging data that steers the
// Ilex Forest Farfetch'd: every step must stand orthogonally beside its bird and
// face it from a side that is not a bounce side. Getting this wrong does not
// fail loudly — the bird simply returns to a solved position and the executor
// burns its whole interaction budget ping-ponging (run-11dd5ya1qev0ry spent
// kicks 3..11 alternating positions 4 and 3, because position 4 staged below
// the bird at (29,23) and FarfetchdPosition4 sends the bird back on UP).
func TestGSIlexBirdStepsFaceForwardBranch(t *testing.T) {
	if len(gsIlexBirdSteps) != 10 {
		t.Fatalf("gsIlexBirdSteps has %d positions, want 10", len(gsIlexBirdSteps))
	}
	for position := 1; position <= 10; position++ {
		step, ok := gsIlexBirdSteps[position]
		if !ok {
			t.Fatalf("gsIlexBirdSteps is missing position %d", position)
		}
		if step.position != position {
			t.Fatalf("gsIlexBirdSteps[%d].position = %d", position, step.position)
		}
		facing, adjacent := facingFromStandToBird(step)
		if !adjacent {
			t.Fatalf("position %d stages at (%d,%d) for bird (%d,%d), which is not orthogonally adjacent",
				position, step.standX, step.standY, step.birdX, step.birdY)
		}
		for _, bounce := range gsIlexBirdBounceFacings[position] {
			if facing == bounce {
				t.Fatalf("position %d stages at (%d,%d) and faces %s at bird (%d,%d), which is a bounce branch in maps/IlexForest.asm",
					position, step.standX, step.standY, facing, step.birdX, step.birdY)
			}
		}
	}
}

// facingFromStandToBird names the facing a player on the staging tile has when
// they look at the bird, and whether the two tiles are orthogonally adjacent.
func facingFromStandToBird(step gsIlexBirdStep) (string, bool) {
	switch {
	case step.standX == step.birdX && step.standY == step.birdY+1:
		return "up", true
	case step.standX == step.birdX && step.standY == step.birdY-1:
		return "down", true
	case step.standX == step.birdX+1 && step.standY == step.birdY:
		return "left", true
	case step.standX == step.birdX-1 && step.standY == step.birdY:
		return "right", true
	}
	return "", false
}
