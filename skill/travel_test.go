package skill_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
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

// TestTravelReplansFromTheWorldAfterEachBattle is the regression test for
// the stale-plan bug: after a battle, Travel must re-read the world from
// RAM and plan the remainder from what it reads there, not resume the plan
// that was being walked when the encounter fired. The post_starter
// checkpoint (Oak's lab, Pallet Town) is the start: from the pallet_town
// checkpoint's frame phase the same grass throws zero encounters
// deterministically (MEASURED, see TestTravelPalletToViridian), so no
// battle — and no re-plan — could be observed. On this walk the battle
// fires at (14,7) on Route 1 and a win leaves the player on that tile, so
// each recorded re-read must name exactly that world, and the journey must
// still arrive.
func TestTravelReplansFromTheWorldAfterEachBattle(t *testing.T) {
	e := fixture.Load(t, "post_starter")
	dest, ok := skill.Place("viridian city")
	if !ok {
		t.Fatal(`Place: "viridian city" not found`)
	}
	res, err := skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 20)
	if err != nil {
		t.Fatalf("Travel: %v; stopped on map %#04x at (%d,%d) after %d battles",
			err, e.Peek8(sym.CurMap), e.Peek8(sym.XCoord), e.Peek8(sym.YCoord), res.Battles)
	}
	if res.Battles < 1 {
		t.Fatalf("premise: Battles = %d, want >= 1 (the walk crosses Route 1's grass)", res.Battles)
	}
	if len(res.Replans) != res.Battles {
		t.Fatalf("Replans = %d, want %d (one re-read after each battle)", len(res.Replans), res.Battles)
	}
	for i, rp := range res.Replans {
		// Every wild battle on this route is on Route 1; a re-read that
		// named any other map would be planning from a stale world.
		if rp.Map != 0x0C {
			t.Errorf("Replans[%d].Map = %#04x, want 0x0C (Route 1)", i, rp.Map)
		}
	}
	// The first battle fires at (14,7) and a win leaves the player there,
	// so the first re-read is exact: the world at that moment.
	if rp := res.Replans[0]; rp.X != 14 || rp.Y != 7 {
		t.Errorf("Replans[0] = (map %#04x, %d, %d), want (0x0C, 14, 7)", rp.Map, rp.X, rp.Y)
	}
	if got := e.Peek8(sym.CurMap); got != 0x01 {
		t.Fatalf("wCurMap = %#04x after the journey, want Viridian City (0x01), at (%d,%d)",
			got, e.Peek8(sym.XCoord), e.Peek8(sym.YCoord))
	}
}

// TestTravelRecoversFromBattleOnWalkToWarp is the regression test for the
// llm-run failure "go to pewter city: ... Traverse: walk to warp on map 0d:
// battle interrupted movement". The route to Pewter leaves Route 2's south
// band by WARP (the (3,43) tile to the forest's south gate), because the
// direct north connection is unwalkable from that band. A wild encounter on
// that walk-TO-A-WARP used to surface as a bare ErrBattleInterrupted, which
// Travel does not recognize as a battle, so the whole run died. The warp leg
// now normalizes to ErrBattle exactly as the connection leg does, so Travel
// fights the encounter and re-plans from where the walk stopped.
//
// The destination is the south gate room (0x32), not Pewter: it is the first
// leg of the Pewter route (the walk to the (3,43) warp, through Route 2's
// grass) and it stops before the forest, whose forced dialogue is a separate
// slice-6 issue (see TestTravelToPewter). From the post_errand checkpoint's
// frame phase the encounter fires at (7,48) on Route 2 (MEASURED), so the
// Battles >= 1 premise holds for this exact walk.
func TestTravelRecoversFromBattleOnWalkToWarp(t *testing.T) {
	e := fixture.Load(t, "post_errand")
	// The south gate room: the (3,43) Route 2 warp lands at (4,7); (5,4) is
	// open floor in the middle of the room, four steps from the landing.
	dest := skill.Destination{Map: 0x32, X: 5, Y: 4}
	res, err := skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 20)
	if err != nil {
		t.Fatalf("Travel to the south gate: %v; stopped on map %#04x at (%d,%d) after %d battles",
			err, e.Peek8(sym.CurMap), e.Peek8(sym.XCoord), e.Peek8(sym.YCoord), res.Battles)
	}
	if res.Battles < 1 {
		t.Fatalf("premise: Battles = %d, want >= 1 (the walk to the gate crosses Route 2's grass)", res.Battles)
	}
	// The battle fired on the walk to the (3,43) warp, on Route 2: every
	// re-read must name Route 2, not the gate or the forest.
	for i, rp := range res.Replans {
		if rp.Map != 0x0D {
			t.Errorf("Replans[%d].Map = %#04x, want 0x0D (Route 2)", i, rp.Map)
		}
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	if p := state.DecodePlayer(&mem); p.MapID != dest.Map || p.X != dest.X || p.Y != dest.Y {
		t.Fatalf("player at (map %#04x, %d, %d), want the south gate (%#04x, %d, %d); Battles=%d",
			p.MapID, p.X, p.Y, dest.Map, dest.X, dest.Y, res.Battles)
	}
	if !state.Controllable(&mem) {
		t.Fatalf("player not controllable at the south gate; Battles=%d BlackedOut=%v", res.Battles, res.BlackedOut)
	}
	t.Logf("reached the south gate after %d battle(s) on the walk to its warp (BlackedOut=%v)", res.Battles, res.BlackedOut)
}

// TestTravelToPewter is the S5b-6 milestone: from the post_errand
// checkpoint (the Oak's-parcel errand is done, so the sleepy old man at
// (19,9) no longer blocks Viridian's north exit, and the player stands
// controllable just south of the gate), Travel crosses the forest route to
// Pewter City and leaves the player exactly at skill.Place("pewter city") —
// the open plaza below the center door warp — still controllable. Every
// expected coordinate comes from that Place, never a literal.
//
// It is a bare Travel call: dialogue recovery (S6-0b) is what lets it
// cross the forest's forced text. It is a full journey — minutes of
// emulation and stochastic wild battles — so it runs only outside -short,
// in the slice's journey command, not the per-task gate. Do not delete it.
func TestTravelToPewter(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; runs in the slice's journey command, not the per-task gate")
	}
	e := fixture.Load(t, "post_errand")
	dest, ok := skill.Place("pewter city")
	if !ok {
		t.Fatal(`Place: "pewter city" not found`)
	}
	// The one-mon party can lose a wild battle on this long route. A blackout
	// is not a failure here: it fully heals the party and warps it to a
	// Pokemon Center, so Travel simply resumes from there. Bounded, like
	// every other retry in this suite.
	var res skill.TravelResult
	var err error
	for attempt := 0; ; attempt++ {
		res, err = skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 20)
		if err == nil || attempt >= 3 {
			break
		}
		if !errors.Is(err, skill.ErrBlackedOut) {
			break
		}
		var zmem state.Mem
		state.Snapshot(e, &zmem)
		zp := state.DecodePlayer(&zmem)
		zlead := state.DecodeParty(&zmem).Mons[0]
		t.Logf("blackout on the way to Pewter (attempt %d): at map %#04x (%d,%d), lead level %d HP %d/%d status=%#02x; waiting for the respawn warp, then resuming",
			attempt+1, zp.MapID, zp.X, zp.Y, zlead.Level, zlead.HP, zlead.MaxHP, zlead.Status)
		settleBlackout(t, e)
	}
	if err != nil {
		t.Fatalf("Travel to pewter city: %v; stopped on map %#04x at (%d,%d) after %d battles",
			err, e.Peek8(sym.CurMap), e.Peek8(sym.XCoord), e.Peek8(sym.YCoord), res.Battles)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	p := state.DecodePlayer(&mem)
	if p.MapID != dest.Map || p.X != dest.X || p.Y != dest.Y {
		t.Fatalf("player at (map %#04x, %d, %d), want Place(pewter city) = (map %#04x, %d, %d); Battles=%d BlackedOut=%v",
			p.MapID, p.X, p.Y, dest.Map, dest.X, dest.Y, res.Battles, res.BlackedOut)
	}
	if !state.Controllable(&mem) {
		t.Fatalf("player not controllable at Pewter; Battles=%d BlackedOut=%v",
			res.Battles, res.BlackedOut)
	}
	t.Logf("reached Pewter City at Place(%s) after %d battles (BlackedOut=%v)", "pewter city", res.Battles, res.BlackedOut)
}

// The movement half of the recovery contract — WalkPath never presses A —
// is pinned structurally: WalkPath and StepOnce send direction buttons only
// (see skill/move.go), and the recovery half (RecoverDialogue presses A on
// ordinary text, never on a choice) is pinned by the deterministic seam
// tests in dialogue_recovery_test.go. A ROM-backed "step onto a sign" test
// is not possible on this ROM: sign tiles are not in the tileset's
// walkable list, so the step is blocked before any box can open (measured
// on Viridian City's signs; 2 of 131 ROM signs are walkable, both map-edge
// cases).

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

