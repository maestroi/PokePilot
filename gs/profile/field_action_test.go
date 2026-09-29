package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func controllableFieldActionMemory() fakeMemory {
	var mem fakeMemory
	mem[sym.MapWidth] = 10
	mem[sym.MapHeight] = 10
	mem[sym.MapStatus] = gen2MapStatusHandle
	return mem
}

func TestDecodeGen2FieldActionState(t *testing.T) {
	mem := controllableFieldActionMemory()
	mem[sym.FacingTileID] = gsCutTreeCollision
	mem[sym.PlayerState] = gsPlayerSurf
	mem[sym.BikeFlags] = gsStrengthActiveBit
	mem[sym.StatusFlags] = gsFlashActiveBit
	mem[sym.FieldMoveSucceeded] = 1

	state := NewGold().DecodeFieldAction(&mem)
	if !state.Controllable {
		t.Fatal("controllable overworld decoded as blocked")
	}
	if !state.CuttableAhead || !state.Surfing || !state.StrengthActive || !state.Lit || !state.ActionSucceeded {
		t.Fatalf("field action state=%+v", state)
	}
	if state.BoulderAhead {
		t.Fatal("boulder target should fail closed until object semantics are implemented")
	}
}

func TestDecodeGen2FieldActionRecognizesAlternateRetailStates(t *testing.T) {
	mem := controllableFieldActionMemory()
	mem[sym.FacingTileID] = gsUnusedCutTreeCollision
	mem[sym.PlayerState] = gsPlayerSurfPikachu

	state := NewSilver().DecodeFieldAction(&mem)
	if !state.CuttableAhead || !state.Surfing {
		t.Fatalf("alternate field states=%+v", state)
	}
}

func TestDecodeGen2FieldActionRequiresExactSuccessValue(t *testing.T) {
	mem := controllableFieldActionMemory()
	mem[sym.FieldMoveSucceeded] = 2
	if state := NewGold().DecodeFieldAction(&mem); state.ActionSucceeded {
		t.Fatalf("non-success field result decoded as success: %+v", state)
	}
}
