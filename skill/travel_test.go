package skill_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
)

// TestTravelPalletTownFightsNothing: from the post_starter checkpoint
// (Oak's lab, the state GetStarter leaves), the walk to Pallet Town crosses
// no grass and must finish with no battles and no blackout. The
// post_starter fixture is the start point, not pallet_town: loading
// pallet_town would make the destination the start and the walk vacuous.
func TestTravelPalletTownFightsNothing(t *testing.T) {
	e := fixture.Load(t, "post_starter")
	dest, ok := skill.Place("pallet town")
	if !ok {
		t.Fatal(`Place: "pallet town" not found`)
	}
	res, err := skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 5)
	if err != nil {
		t.Fatalf("Travel: %v", err)
	}
	if res.Battles != 0 {
		t.Errorf("Battles = %d, want 0", res.Battles)
	}
	if res.BlackedOut {
		t.Error("BlackedOut = true, want false")
	}
}

// TestTravelMaxBattlesZero: the bound is checked before anything walks.
func TestTravelMaxBattlesZero(t *testing.T) {
	e := loadFixture(t)
	before := playerAt(t, e)
	dest, ok := skill.Place("pallet town")
	if !ok {
		t.Fatal(`Place: "pallet town" not found`)
	}
	_, err := skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 0)
	if err == nil {
		t.Fatal("Travel with maxBattles 0 = nil, want an error mentioning the bound")
	}
	if !strings.Contains(err.Error(), "maxBattles") {
		t.Errorf("err = %q, want it to mention maxBattles", err)
	}
	after := playerAt(t, e)
	if after.MapID != before.MapID || after.X != before.X || after.Y != before.Y {
		t.Errorf("player moved: before = map %#04x (%d,%d), after = map %#04x (%d,%d)",
			before.MapID, before.X, before.Y, after.MapID, after.X, after.Y)
	}
}

// TestTravelNonsenseDestination: a non-battle failure must come back
// unchanged and must NOT be reported as a battle problem. This is the
// errors.Is discrimination in Travel's second step.
func TestTravelNonsenseDestination(t *testing.T) {
	e := loadFixture(t)
	// Map 0xFF is above the ROM's highest map id, so it is not a node in
	// the graph and no route can reach it: a genuine no-route failure.
	dest := skill.Destination{Map: 0xFF, X: 0, Y: 0}
	res, err := skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 5)
	if err == nil {
		t.Fatal("Travel to map 0xff = nil, want a no-route error")
	}
	if errors.Is(err, skill.ErrBattle) {
		t.Fatalf("err = %v, must not be reported as a battle problem", err)
	}
	if !errors.Is(err, world.ErrNoRoute) {
		t.Fatalf("err = %v, want the underlying world.ErrNoRoute to come back unchanged", err)
	}
	if res.Battles != 0 {
		t.Errorf("Battles = %d, want 0", res.Battles)
	}
}

// TestTravelPalletToViridian is the milestone: from the post-story state,
// Travel walks through Pallet Town, crosses Route 1's tall grass, fights
// the wild encounters, and reaches Viridian City. A plain GoTo on this
// route MEASURED stops with "Traverse: battle on map 0c at (14,7): battle
// interrupted the route" roughly 760 frames into Route 1; Travel must get
// past it. Setup is the cached post_starter checkpoint: the battles >= 1
// assertion is tied to the wRandom phase of this exact walk, and the
// post_starter fixture is the state GetStarter left (in Oak's lab). The
// pallet_town checkpoint is NOT a drop-in start here: from the town entry
// the same grass throws zero encounters, deterministically (MEASURED), and
// the assertion could not hold.
func TestTravelPalletToViridian(t *testing.T) {
	e := fixture.Load(t, "post_starter")
	dest, ok := skill.Place("viridian city")
	if !ok {
		t.Fatal(`Place: "viridian city" not found`)
	}
	res, err := skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 20)
	if err != nil {
		t.Fatalf("Travel to viridian city: %v; stopped on map %#04x at (%d,%d) after %d battles",
			err, e.Peek8(sym.CurMap), e.Peek8(sym.XCoord), e.Peek8(sym.YCoord), res.Battles)
	}
	t.Logf("reached Viridian City after %d battles (BlackedOut=%v)", res.Battles, res.BlackedOut)
	if res.Battles < 1 {
		t.Errorf("Battles = %d, want >= 1 (Route 1's grass must have fought at least once)", res.Battles)
	}
	if got := e.Peek8(sym.CurMap); got != 0x01 {
		t.Fatalf("wCurMap = %#04x, want Viridian City (0x01), at (%d,%d)",
			got, e.Peek8(sym.XCoord), e.Peek8(sym.YCoord))
	}
}
