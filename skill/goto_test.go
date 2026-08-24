package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// TestGoToViridianPokecenter is the slice-2 milestone: from Red's bedroom,
// GoTo walks the player to the Viridian Pokémon Center, crossing Red's house,
// Pallet Town, Route 1 and Viridian City, then the player faces the nurse and
// talks.
func TestGoToViridianPokecenter(t *testing.T) {
	e := loadFixture(t)

	dest, ok := skill.Place("viridian pokemon center")
	if !ok {
		t.Fatal("Place: \"viridian pokemon center\" not found")
	}
	if dest.Map != 0x29 || dest.X != 3 || dest.Y != 2 {
		t.Fatalf("Place = %+v, want {Map:0x29 X:3 Y:2}", dest)
	}

	if err := skill.GoTo(e, e.ROM(), dest); err != nil {
		t.Fatalf("GoTo: %v", err)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	p := state.DecodePlayer(&mem)
	if p.MapID != 0x29 {
		t.Fatalf("CurMap = %#04x, want 0x29", p.MapID)
	}
	if p.X != 3 || p.Y != 2 {
		t.Errorf("player = (%d,%d), want (3,2)", p.X, p.Y)
	}
	if !state.Controllable(&mem) {
		t.Error("player not controllable after GoTo")
	}

	// The nurse stands at (3,1); the player is at (3,2) facing her.
	if err := skill.Face(e, 3, 1); err != nil {
		t.Fatalf("Face(3,1): %v", err)
	}
	presses, err := skill.Talk(e)
	if err != nil {
		t.Fatalf("Talk: %v", err)
	}
	if presses < 1 {
		t.Errorf("Talk presses = %d, want >= 1", presses)
	}
}
