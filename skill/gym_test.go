package skill_test

import (
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
)

// gymLeadLevel is the level the lead must reach before Pewter. Two fights
// stand in the way: the rival's forced cutscene battle on the forest leg
// (the player blacked out at level 8) and Brock himself, whose team is a
// level 12 Geodude and a level 14 Onix. The forest grass is sparse (about
// one battle per thirty steps), so the target stays within reach of a
// single training loop; a water lead at 12 is super-effective against the
// rock team and clears both. If a battle still loses, this is the knob.
const gymLeadLevel = 12

// travelFightsThrough is Travel plus the one case Travel misses: a wild
// battle that interrupts the walk inside a Traverse warp leg is not
// normalized to ErrBattle (Traverse normalizes connection legs only), so
// Travel returns it as a plain error with the battle still in progress.
// Fight it, then let Travel re-plan from where the walk stopped.
func travelFightsThrough(t *testing.T, e *emu.Emu, romData []byte, dest skill.Destination, policy skill.MovePolicy, maxBattles int) {
	t.Helper()
	for i := 0; ; i++ {
		_, err := skill.Travel(e, romData, dest, policy, maxBattles)
		if err == nil {
			return
		}
		var mem state.Mem
		state.Snapshot(e, &mem)
		pos := fmt.Sprintf("map %#04x at (%d,%d)", mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
		switch {
		case state.DecodeBattle(&mem) != nil:
			if i >= 10 {
				t.Fatalf("Travel to (%#04x,%d,%d): still interrupted by battles after %d: %v", dest.Map, dest.X, dest.Y, i, err)
			}
			outcome, berr := skill.Battle(e, policy)
			if berr != nil {
				t.Fatalf("wild battle on the way to (%#04x,%d,%d): %v", dest.Map, dest.X, dest.Y, berr)
			}
			if outcome == state.ResultLost {
				t.Logf("wild battle on the way to (%#04x,%d,%d) ended in a blackout", dest.Map, dest.X, dest.Y)
			}
		case state.DecodeDialogue(&mem) != nil:
			// An NPC the path walks into opens a text box; WalkPath reports it
			// as ErrDialogueInterrupted and walkAround does not recover from it
			// (it only re-plans around hard blocks). Page the box closed and let
			// Travel re-plan from where the walk stopped.
			if i >= 10 {
				t.Fatalf("Travel to (%#04x,%d,%d): still interrupted by a text box after %d retries at %s: %v", dest.Map, dest.X, dest.Y, i, pos, err)
			}
			ds := state.DecodeDialogue(&mem)
			t.Logf("text box at %s: fontLoaded=%#04x joyIgnore=%#04x walkCounter=%#04x battle=%v text=%q", pos,
				mem.U8(sym.FontLoaded), mem.U8(sym.JoyIgnore), mem.U8(sym.WalkCounter), state.DecodeBattle(&mem) != nil, ds.Text)
			dismissDialogue(t, e)
		default:
			t.Fatalf("Travel to (%#04x,%d,%d): %v", dest.Map, dest.X, dest.Y, err)
		}
	}
}

// dismissDialogue lets an open text box run to its end. A plain NPC line
// pages closed under a held A; the rival's "hey, wait up" cutscene is forced
// and plays out on its own, so step frames (holding A, which is harmless for
// both) until either the box is gone and the player is controllable, or a
// battle the box led into is in progress — the caller then handles it.
func dismissDialogue(t *testing.T, e *emu.Emu) {
	t.Helper()
	for i := 0; i < 400; i++ {
		// Text pages on a button press, not a hold, so tap A rather than
		// holding it. The tap is harmless for a forced cutscene and required
		// for an NPC line.
		e.Tap(emu.A, 3, 7)
		var mem state.Mem
		state.Snapshot(e, &mem)
		if state.DecodeBattle(&mem) != nil {
			return
		}
		if state.DecodeDialogue(&mem) == nil && state.Controllable(&mem) {
			return
		}
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	t.Fatalf("dismissDialogue: text box did not settle: fontLoaded=%#04x joyIgnore=%#04x battle=%v text=%q",
		mem.U8(sym.FontLoaded), mem.U8(sym.JoyIgnore), state.DecodeBattle(&mem) != nil, state.DecodeDialogue(&mem).Text)
}

// crossGate steps the player out of a forest gate map. This is the one
// crossing GoTo and Traverse cannot do: each gate's ROM warp table points
// GoTo at (4,0), which this ROM marks non-walkable (measured), so the
// automatic path reaches the gate and stops; the real exit is (5,0), one
// tile right. The gate is 10x8 tiles with its walkable corridor at x 4-5,
// so the approach is unambiguous: walk to (5,1) and hold up onto (5,0).
//
// The warp writes wCurMap before the destination coordinates, so the first
// snapshot after the map change still carries the gate's (5,0). The south
// gate lands at forest (17,47) and the north gate at Route 2's north band;
// both are walkable, so "standing on a walkable tile of the target map" is
// the settle predicate: it is true only once the coordinates have been
// written for the new map.
func crossGate(t *testing.T, e *emu.Emu, romData []byte, fromMap, toMap uint8) {
	t.Helper()
	if cur := e.Peek8(sym.CurMap); cur != fromMap {
		t.Fatalf("crossGate: on map %#04x, want the gate %#04x", cur, fromMap)
	}
	if err := skill.GoTo(e, romData, skill.Destination{Map: fromMap, X: 5, Y: 1}); err != nil {
		t.Fatalf("crossGate %#04x: walk to (5,1): %v", fromMap, err)
	}
	if _, err := e.HoldUntil(emu.Up, 600, func(m *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return mem.U8(sym.CurMap) != fromMap
	}); err != nil {
		t.Fatalf("crossGate %#04x: holding up from (5,1) did not cross: %v", fromMap, err)
	}
	h, err := rom.ParseMap(romData, toMap)
	if err != nil {
		t.Fatalf("crossGate %#04x: parse map %#04x: %v", fromMap, toMap, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("crossGate %#04x: build map %#04x: %v", fromMap, toMap, err)
	}
	var mem state.Mem
	for i := 0; ; i++ {
		e.StepFrame()
		state.Snapshot(e, &mem)
		// The transition writes wCurMap, then the destination coordinates,
		// and only then clears wJoyIgnore; all three must hold before the
		// next leg reads a position.
		if mem.U8(sym.CurMap) == toMap &&
			grid.Walkable(int(mem.U8(sym.XCoord)), int(mem.U8(sym.YCoord))) &&
			state.Controllable(&mem) {
			break
		}
		if i >= 1200 {
			t.Fatalf("crossGate %#04x: never settled on map %#04x: at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
				fromMap, toMap, mem.U8(sym.XCoord), mem.U8(sym.YCoord), mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
		}
	}
	if !state.Controllable(&mem) {
		t.Fatalf("crossGate %#04x: not controllable after crossing: at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			fromMap, mem.U8(sym.XCoord), mem.U8(sym.YCoord), mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	t.Logf("crossed gate %#04x -> %#04x, landed at (%d,%d)", fromMap, toMap, mem.U8(sym.XCoord), mem.U8(sym.YCoord))
}

// TestGymBoulderBadge is the journey milestone: from the fresh post_errand
// checkpoint (the player has just delivered Oak's parcel and is back in
// Viridian City) it travels to the Pewter Gym, trains the lead first if it
// is under the level that beats Brock, fights the gym, and proves the
// Boulder Badge is set in RAM with the player controllable again.
//
// The route has five travel legs and two hand-driven gate crossings. The
// gates (maps 0x32 south, 0x2F north) are small bridge maps between Route
// 2 and the forest, and their ROM warp table points the pathfinder at
// (4,0), a non-walkable tile in this ROM, so no automatic leg can cross
// them: crossGate walks to (5,1) and holds up onto the real (5,0) warp.
//
//   1. 0x01 -> 0x32 (5,1)   city to Route 2, up the band to the south gate
//   2. crossGate           south gate -> forest
//   3. 0x33 -> 0x2F (5,1)   through the forest to the north gate
//   4. crossGate           north gate -> Route 2's north band
//   5. 0x0D -> gym         up Route 2 into Pewter City, through the door
func TestGymBoulderBadge(t *testing.T) {
	e := fixture.Load(t, "post_errand")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	// Leg 1: Viridian City to the south gate, stopping one row below the
	// (5,0) forest warp. The gate is entered from Route 2's warp (3,43).
	southGate := skill.Destination{Map: 0x32, X: 5, Y: 1}
	travelFightsThrough(t, e, romData, southGate, policy, 10)

	// Leg 2: the south gate into the forest.
	crossGate(t, e, romData, 0x32, 0x33)

	// The gate drops the player at (17,47), inside the pocket that holds the
	// four south warps (15,47)-(18,47). A grind ping-pong there steps on a
	// warp and gets carried back to Route 2's dead-end south band, so walk up
	// into the open forest first: the grass the trainer finds is a few steps
	// from here, and it is clear of every warp on the map.
	safeSpot := skill.Destination{Map: 0x33, X: 17, Y: 40}
	travelFightsThrough(t, e, romData, safeSpot, policy, 5)

	// Train in the forest itself if the lead is under the level that
	// beats Brock: its grass is a few steps from the travel target, so the
	// detour is the approach the journey already made.
	var mem state.Mem
	state.Snapshot(e, &mem)
	if lead := state.DecodeParty(&mem).Mons[0].Level; int(lead) < gymLeadLevel {
		res, err := skill.Train(e, romData, gymLeadLevel, policy, 25)
		if err != nil {
			t.Fatalf("Train: %v (start=%d end=%d battles=%d, reached=%v, blackedOut=%v)", err, res.StartLevel, res.EndLevel, res.Battles, res.Reached, res.BlackedOut)
		}
		state.Snapshot(e, &mem)
		if lead := state.DecodeParty(&mem).Mons[0].Level; int(lead) < gymLeadLevel {
			t.Fatalf("the lead is level %d after %d battles, want >= %d to face Brock (blackedOut=%v)",
				lead, res.Battles, gymLeadLevel, res.BlackedOut)
		}
		t.Logf("trained the lead to level %d in %d battles", state.DecodeParty(&mem).Mons[0].Level, res.Battles)
	}
	state.Snapshot(e, &mem)
	t.Logf("post-train position: map=%#04x at (%d,%d) controllable=%v",
		mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), state.Controllable(&mem))

	// Leg 3: the forest to the north gate.
	northGate := skill.Destination{Map: 0x2F, X: 5, Y: 1}
	travelFightsThrough(t, e, romData, northGate, policy, 10)

	// Leg 4: the north gate into Route 2's north band.
	crossGate(t, e, romData, 0x2F, 0x0D)

	// Leg 5: up Route 2 into Pewter City and through the gym door.
	gym, ok := skill.Place("pewter gym")
	if !ok {
		t.Fatalf("Place \"pewter gym\" not found")
	}
	travelFightsThrough(t, e, romData, gym, policy, 10)

	outcome, err := skill.Gym(e, romData, policy)
	if err != nil {
		t.Fatalf("Gym: %v", err)
	}
	if outcome != state.ResultWon {
		t.Fatalf("Gym outcome = %d, want ResultWon", int(outcome))
	}

	// Gym's postcondition is the badge itself, so this poll is expected to
	// succeed on the first snapshot; it stays as the trip wire that would
	// catch a Gym that returned before the write landed.
	for i := 0; i < 3000; i++ {
		e.StepFrame()
		state.Snapshot(e, &mem)
		if mem.U8(sym.ObtainedBadges)&0x01 != 0 {
			break
		}
	}
	state.Snapshot(e, &mem)
	if mem.U8(sym.ObtainedBadges)&0x01 == 0 {
		t.Fatalf("wObtainedBadges = %#02x after the gym, want bit 0 (Boulder) set: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.ObtainedBadges), mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	if !state.Controllable(&mem) {
		t.Fatalf("player not controllable after the gym: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	t.Logf("first badge won: lead level %d, wObtainedBadges=%#02x", state.DecodeParty(&mem).Mons[0].Level, mem.U8(sym.ObtainedBadges))
}
