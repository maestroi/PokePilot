package skill_test

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestUseFieldItemPotion is S8-5's postcondition: from the overworld, with a
// damaged lead and a POTION in the bag, UseFieldItem raises the lead's
// current HP — read from RAM before and after.
//
// Setup: the viridian_mart fixture (post-errand, so the start menu carries
// the POKéDEX entry and ITEM sits at index 2, not 1) walks into Viridian
// Forest and takes the hidden POTION at (1,18) — the only potion reachable
// this early in the story: no shop before Pewter stocks it
// (ViridianMartClerkText: POKE_BALL/ANTIDOTE/PARLYZ_HEAL/BURN_HEAL) and the
// forest's item ball at (12,29) is walled off. A wild battle on Route 1 then
// damages the lead — a successful Gen 1 escape costs no damage (the enemy
// never gets its turn), so the battle is fought, not fled. The postcondition
// is the HP RISING, not "a text box appeared": UseFieldItem itself reads
// DecodeParty before and after and fails with ErrFieldItemNoEffect when the
// effect did not land.
func TestUseFieldItemPotion(t *testing.T) {
	if testing.Short() {
		t.Skip("emulator journey test")
	}
	m := fixture.Load(t, "viridian_mart")
	policy := skill.StatAwareMove(m.ROM())

	// Hidden POTION at (1,18) on map 0x33. Hidden events fire on A pressed
	// while FACING the target tile (CheckForHiddenEvent ->
	// CheckIfCoordsInFrontOfPlayerMatch, engine/overworld/hidden_events.asm),
	// not on stepping onto it: walk to the corridor tile above it, face it,
	// Talk it.
	forest, ok := skill.Place("viridian forest")
	if !ok {
		t.Fatal(`Place "viridian forest" not found`)
	}
	travelLeg(t, m, policy, forest, "the forest")
	travelLeg(t, m, policy, skill.Destination{Map: 0x33, X: 1, Y: 17}, "(1,17) on the forest corridor")
	if err := skill.Face(m, 1, 18); err != nil {
		t.Fatalf("face the hidden item tile: %v", err)
	}
	if _, err := skill.Talk(m); err != nil {
		t.Fatalf("talk to the hidden item: %v", err)
	}
	if q := bagQty(t, m, itemPotion); q != 1 {
		t.Fatalf("precondition: bag POTION = %d after the hidden item, want 1", q)
	}

	r1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal(`Place "route 1" not found`)
	}
	travelLeg(t, m, policy, r1, "Route 1")
	damageLead(t, m, policy, r1)

	var before state.Mem
	state.Snapshot(m, &before)
	if !state.Controllable(&before) {
		t.Fatal("precondition: player not controllable after the damage battle")
	}
	lead := state.DecodeParty(&before).Mons[0]
	if lead.HP == 0 || lead.HP >= lead.MaxHP {
		t.Fatalf("precondition: lead not damaged (HP %d/%d)", lead.HP, lead.MaxHP)
	}

	if err := skill.UseFieldItem(m, itemPotion, 0); err != nil {
		t.Fatalf("UseFieldItem(POTION, 0): %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	leadAfter := state.DecodeParty(&after).Mons[0]
	if leadAfter.HP <= lead.HP {
		t.Fatalf("postcondition: lead HP did not rise: %d -> %d (max %d)", lead.HP, leadAfter.HP, lead.MaxHP)
	}
	if got := bagQty(t, m, itemPotion); got != 0 {
		t.Errorf("postcondition: bag POTION = %d, want 0 (the one was used)", got)
	}
	if !state.Controllable(&after) {
		t.Error("postcondition: player is not controllable after using the item")
	}
}

// damageLead fights wild battles on Route 1 until the lead is damaged and
// alive. A win leaves it wounded (the enemy attacked at least once on a
// multi-hit KO); a loss blackouts — the party is fully healed and respawned
// on Pallet Town, which has no center — so the leg back to Route 1 is paid
// again. Bounded: four battles is far more than the L7 lead needs against
// Pidgey.
func damageLead(t *testing.T, m *emu.Emu, policy skill.MovePolicy, r1 skill.Destination) {
	t.Helper()
	for tries := 0; tries < 4; tries++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if lead := state.DecodeParty(&mem).Mons[0]; lead.HP > 0 && lead.HP < lead.MaxHP {
			return
		}
		if m.Peek8(sym.CurMap) != r1.Map {
			travelLeg(t, m, policy, r1, "Route 1")
		}
		if err := skill.EnterWildBattle(m, 3); err != nil {
			t.Fatalf("enter wild battle (try %d): %v", tries+1, err)
		}
		outcome, err := skill.Battle(m, policy)
		if err != nil {
			t.Fatalf("battle (try %d): %v", tries+1, err)
		}
		if outcome == state.ResultLost {
			t.Logf("lost the damage battle (try %d); settling the blackout", tries+1)
			settleBlackout(t, m)
		}
	}
	// The loop's exit condition is re-checked by the caller's precondition.
}

// travelLeg runs one Travel leg with bounded blackout retries: the
// post-errand lead is level 7, so a lost wild battle on the grass legs is an
// ordinary outcome — a blackout fully heals the party and warps it to the
// last town (a Route 1 blackout lands on Pallet Town, which has no center),
// and Travel resumes from there. The retry must settle the respawn warp
// first: fixture.Travel's immediate re-attempt plans from the mid-flip
// position and can wedge (measured: "step down blocked" at the respawn
// tile). The same pattern TestTravelToPewter uses.
func travelLeg(t *testing.T, m *emu.Emu, policy skill.MovePolicy, dest skill.Destination, label string) {
	t.Helper()
	for attempt := 0; ; attempt++ {
		_, err := skill.Travel(m, m.ROM(), dest, policy, 20)
		if err == nil || attempt >= 3 || !errors.Is(err, skill.ErrBlackedOut) {
			if err != nil {
				t.Fatalf("travel to %s: %v", label, err)
			}
			return
		}
		t.Logf("blackout on the way to %s (attempt %d); settling the respawn", label, attempt+1)
		settleBlackout(t, m)
	}
}
