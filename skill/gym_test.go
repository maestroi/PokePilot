package skill_test

import (
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// gymLeadLevel is the level the lead must reach before Brock, whose team
// is a level 12 Geodude and a level 14 Onix. A level 8 Squirtle with
// BUBBLE is derived to win that matchup; if that is wrong the battle
// below fails and this constant is the knob.
const gymLeadLevel = 8

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

// dismissDialogue holds A until no dialogue is active, paging through
// multi-line NPC text and closing the box.
func dismissDialogue(t *testing.T, e *emu.Emu) {
	t.Helper()
	if _, err := e.HoldUntil(emu.A, 600, func(m *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return state.DecodeDialogue(&mem) == nil
	}); err != nil {
		t.Fatalf("dismissDialogue: text box did not close: %v", err)
	}
}

// TestGymBoulderBadge is the journey milestone: from the fresh post_errand
// checkpoint (the player has just delivered Oak's parcel and is back in
// Viridian City) it travels to the Pewter Gym, trains the lead first if it
// is under the level that beats Brock, fights the gym, and proves the
// Boulder Badge is set in RAM with the player controllable again.
//
// The route has four travel legs. The gates (maps 0x32 south, 0x2F north)
// are small bridge maps between Route 2 and the forest, and each exposes
// two warp tiles to the same destination: (4,0) is non-walkable in this
// ROM and the pathfinder's only route to it crosses the (5,0) warp, so
// Traverse selects the reachable (5,0) tile from where the player stands.
//
//  1. 0x01 -> 0x32 (5,1)    city to Route 2, up the band to the south gate
//  2. 0x32 -> 0x33 (17,43)  south gate -> forest, the training ground
//  3. 0x33 -> 0x2F (5,1)    through the forest to the north gate
//  4. 0x2F -> gym           north gate -> Route 2's north band -> Pewter City, through the door
func TestGymBoulderBadge(t *testing.T) {
	e := fixture.Load(t, "post_errand")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	// Leg 1: Viridian City to the south gate, stopping one row below the
	// (5,0) forest warp. The gate is entered from Route 2's warp (3,43).
	southGate := skill.Destination{Map: 0x32, X: 5, Y: 1}
	travelFightsThrough(t, e, romData, southGate, policy, 10)

	// Leg 2: the south gate into the forest. Traverse picks the reachable
	// (5,0) warp tile itself; the landing (17,47) is the training ground's
	// own row, so the detour Train makes is the approach the journey
	// already took.
	forest, ok := skill.Place("viridian forest")
	if !ok {
		t.Fatalf(`Place "viridian forest" not found`)
	}
	travelFightsThrough(t, e, romData, forest, policy, 10)

	// Train in the forest itself if the lead is under the level that
	// beats Brock: its grass is a few steps from the travel target, so the
	// detour is the approach the journey already made.
	var mem state.Mem
	state.Snapshot(e, &mem)
	if lead := state.DecodeParty(&mem).Mons[0].Level; int(lead) < gymLeadLevel {
		res, err := skill.Train(e, romData, gymLeadLevel, policy, 20)
		if err != nil {
			t.Fatalf("Train: %v (battles=%d, reached=%v, blackedOut=%v)", err, res.Battles, res.Reached, res.BlackedOut)
		}
		state.Snapshot(e, &mem)
		if lead := state.DecodeParty(&mem).Mons[0].Level; int(lead) < gymLeadLevel {
			t.Fatalf("the lead is level %d after %d battles, want >= %d to face Brock (blackedOut=%v)",
				lead, res.Battles, gymLeadLevel, res.BlackedOut)
		}
		t.Logf("trained the lead to level %d in %d battles", state.DecodeParty(&mem).Mons[0].Level, res.Battles)
	}

	// Leg 3: the forest to the north gate.
	northGate := skill.Destination{Map: 0x2F, X: 5, Y: 1}
	travelFightsThrough(t, e, romData, northGate, policy, 10)

	// Leg 4: the north gate into Route 2's north band, then up Route 2
	// into Pewter City and through the gym door. The gate crossing is an
	// ordinary leg: Traverse picks the reachable (5,0) warp tile itself.
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
