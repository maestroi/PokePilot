package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// TestGoToViridianPokecenter is the slice-2 milestone: from Red's bedroom,
// the player walks to the Viridian Pokémon Center, crossing Red's house,
// Pallet Town, Route 1 and Viridian City, then the player faces the nurse and
// talks. The name keeps "GoTo" as the milestone's identity even though the
// walk now uses Travel: the route crosses Route 1's tall grass, GoTo aborts
// on a wild battle by design (MEASURED ~1 encounter on the Pallet ->
// Viridian leg), and Travel fights it and resumes, so the test is
// deterministic.
func TestGoToViridianPokecenter(t *testing.T) {
	e := loadFixture(t)
	if err := skill.GetStarter(e, e.ROM(), skill.StarterSquirtle, skill.StatAwareMove(e.ROM())); err != nil {
		t.Fatalf("GetStarter: %v", err)
	}

	dest, ok := skill.Place("viridian pokemon center")
	if !ok {
		t.Fatal("Place: \"viridian pokemon center\" not found")
	}
	if dest.Map != 0x29 || dest.X != 3 || dest.Y != 3 {
		t.Fatalf("Place = %+v, want {Map:0x29 X:3 Y:3}", dest)
	}

	res, err := skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 20)
	if err != nil {
		t.Fatalf("Travel: %v", err)
	}
	t.Logf("reached the Viridian Pokémon Center after %d battles (BlackedOut=%v)", res.Battles, res.BlackedOut)

	var mem state.Mem
	state.Snapshot(e, &mem)
	p := state.DecodePlayer(&mem)
	if p.MapID != 0x29 {
		t.Fatalf("CurMap = %#04x, want 0x29", p.MapID)
	}
	if p.X != 3 || p.Y != 3 {
		t.Errorf("player = (%d,%d), want (3,3)", p.X, p.Y)
	}
	if !state.Controllable(&mem) {
		t.Error("player not controllable after GoTo")
	}

	// The nurse stands at (3,1) behind the counter at (3,2). The player
	// cannot stand on a counter tile, so the interaction is to stand below
	// it at (3,3) and face the counter; the game reaches the nurse across
	// it. Talk asserts a text box actually opened, so its success is what
	// proves she responded.
	if err := skill.Face(e, 3, 2); err != nil {
		t.Fatalf("Face(3,2): %v", err)
	}
	presses, err := skill.Talk(e)
	if err != nil {
		t.Fatalf("Talk: %v", err)
	}
	if presses < 1 {
		t.Errorf("Talk presses = %d, want >= 1", presses)
	}
}
