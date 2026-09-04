package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestPPRestoreMoveSlotPrefersExhaustedMove(t *testing.T) {
	mon := state.Mon{
		Moves: [4]uint8{1, 2, 3, 0},
		PP:    [4]uint8{8, 0, 3, 0},
	}
	slot, ok := ppRestoreMoveSlot(mon)
	if !ok || slot != 1 {
		t.Fatalf("ppRestoreMoveSlot = %d,%v; want 1,true", slot, ok)
	}
}

func TestPPRestoreMoveSlotFallsBackToLowestPP(t *testing.T) {
	mon := state.Mon{
		Moves: [4]uint8{1, 2, 3, 0},
		PP:    [4]uint8{8, 2, 3, 0},
	}
	slot, ok := ppRestoreMoveSlot(mon)
	if !ok || slot != 1 {
		t.Fatalf("ppRestoreMoveSlot = %d,%v; want 1,true", slot, ok)
	}
}

func TestFieldItemHadEffectAcceptsPPIncrease(t *testing.T) {
	before := state.Mon{Moves: [4]uint8{1}, PP: [4]uint8{0}, HP: 20, MaxHP: 20}
	after := before
	after.PP[0] = 10
	if !fieldItemHadEffect(before, after) {
		t.Fatal("PP increase was not accepted as a positive field-item effect")
	}
}

func TestFieldItemHadEffectRejectsNoChange(t *testing.T) {
	mon := state.Mon{Moves: [4]uint8{1}, PP: [4]uint8{10}, HP: 20, MaxHP: 20}
	if fieldItemHadEffect(mon, mon) {
		t.Fatal("unchanged target was accepted as a field-item effect")
	}
}
