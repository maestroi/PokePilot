package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestLeagueItemIDsMatchGen1Constants(t *testing.T) {
	if itemRevive != 0x35 || itemMaxRevive != 0x36 {
		t.Fatalf("revive ids = %#02x/%#02x, want 0x35/0x36", itemRevive, itemMaxRevive)
	}
	if itemEther != 0x50 || itemMaxEther != 0x51 || itemElixer != 0x52 || itemMaxElixer != 0x53 {
		t.Fatalf("PP restore ids = %#02x/%#02x/%#02x/%#02x, want 0x50..0x53", itemEther, itemMaxEther, itemElixer, itemMaxElixer)
	}
}

func TestPPRestoreMoveSlotPrefersExhaustedMove(t *testing.T) {
	mon := state.Mon{
		Moves: [4]uint8{1, 2, 3, 0},
		PP:    [4]uint8{5, 0, 1, 0},
	}
	slot, ok := ppRestoreMoveSlot(mon)
	if !ok || slot != 1 {
		t.Fatalf("ppRestoreMoveSlot = (%d,%v), want exhausted move slot 1", slot, ok)
	}
}
