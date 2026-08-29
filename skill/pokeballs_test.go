package skill_test

import (
	"testing"
	"time"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// trainForRoute22Rival levels the lead to skill.Route22RivalLeadLevel on
// Route 1's grass, healing at the Viridian center before every chunk of two
// levels. The lead takes cumulative damage across battles — measured
// 2026-08-28: a fully healed level-7 lead won eight straight fights and
// lost the ninth to a level-3 Pidgey on the damage it had accumulated —
// and with a one-mon party that loss is a blackout. Two-level chunks keep
// the damage between heals survivable; a blackout mid-chunk is still
// recoverable, because the respawn fully heals and the next chunk travels
// back.
func trainForRoute22Rival(t *testing.T, m *emu.Emu, romData []byte, policy skill.MovePolicy) {
	t.Helper()
	center, ok := skill.Place("viridian pokemon center")
	if !ok {
		t.Fatal(`Place "viridian pokemon center" not found`)
	}
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal(`Place "route 1" not found`)
	}
	for {
		var mem state.Mem
		state.Snapshot(m, &mem)
		lead := int(state.DecodeParty(&mem).Mons[0].Level)
		if lead >= skill.Route22RivalLeadLevel {
			t.Logf("lead ready at level %d", lead)
			return
		}
		if _, err := skill.Travel(m, romData, center, policy, 20); err != nil {
			t.Fatalf("Travel to the center: %v", err)
		}
		if err := skill.Heal(m); err != nil {
			t.Fatalf("Heal: %v", err)
		}
		if _, err := skill.Travel(m, romData, route1, policy, 20); err != nil {
			t.Fatalf("Travel to route 1: %v", err)
		}
		res, err := skill.Train(m, romData, lead+2, policy, 12)
		if err != nil {
			t.Fatalf("Train: %v (start=%d end=%d battles=%d reached=%v blackedOut=%v)",
				err, res.StartLevel, res.EndLevel, res.Battles, res.Reached, res.BlackedOut)
		}
		t.Logf("chunk: lead %d -> %d in %d battles (blackedOut=%v)",
			res.StartLevel, res.EndLevel, res.Battles, res.BlackedOut)
	}
}

// pokeballCount is the total POKE_BALL quantity in the decoded bag. The
// postcondition of GetPokeBalls is 5, and it must come from
// state.DecodeInventory, not from the story flag: CheckAndSetEvent sets
// EVENT_GOT_POKEBALLS_FROM_OAK BEFORE GiveItem runs, so an event-only
// assertion passes against an empty bag.
func pokeballCount(mem *state.Mem) int {
	n := 0
	for _, it := range state.DecodeInventory(mem).Items {
		if it.ID == skill.ItemPokeBall {
			n += int(it.Quantity)
		}
	}
	return n
}

// TestGetPokeBalls is S6-1: from the post-parcel state, fight the Route 22
// rival (a coordinate trigger at (29,5), not an NPC talk) and get the five
// POKE_BALLs from Oak. It crosses Route 1's grass and fights a trainer
// battle, so it is a full journey: minutes of emulation and stochastic wild
// battles, run only outside -short, in the slice's journey command, not the
// per-task gate.
func TestGetPokeBalls(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey (Route 22 rival battle); runs in the slice's journey command, not the per-task gate")
	}
	m := fixture.Load(t, "post_errand")
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)

	// Preconditions: the post_errand state has none of the balls' results
	// yet, so the test proves GetPokeBalls produces each one.
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.HasEvent(&mem, state.EventGotPokeballsFromOak) {
		t.Fatal("precondition: pokeballs-from-Oak flag already set in post_errand fixture")
	}
	if state.HasEvent(&mem, state.EventBeatRoute22Rival1stBattle) {
		t.Fatal("precondition: route 22 rival battle event already set in post_errand fixture")
	}
	if n := pokeballCount(&mem); n != 0 {
		t.Fatalf("precondition: %d POKE_BALL already in bag: %+v", n, state.DecodeInventory(&mem).Items)
	}

	// The post-parcel lead (level 7) loses the rival battle — his party is
	// Pidgey Lv9 + Bulbasaur Lv8 — so train on Route 1's grass first;
	// GetPokeBalls itself does not train.
	trainForRoute22Rival(t, m, romData, policy)

	start := time.Now()
	if err := skill.GetPokeBalls(m, romData, policy); err != nil {
		t.Fatalf("GetPokeBalls: %v", err)
	}
	t.Logf("GetPokeBalls took %v", time.Since(start))

	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, state.EventBeatRoute22Rival1stBattle) {
		t.Fatalf("postcondition: %s not set", state.EventBeatRoute22Rival1stBattle)
	}
	if !state.HasEvent(&mem, state.EventGotPokeballsFromOak) {
		t.Fatalf("postcondition: %s not set", state.EventGotPokeballsFromOak)
	}
	if n := pokeballCount(&mem); n != 5 {
		t.Fatalf("postcondition: bag holds %d POKE_BALL, want 5: %+v", n, state.DecodeInventory(&mem).Items)
	}
	if !state.Controllable(&mem) {
		t.Fatalf("postcondition: player not controllable: %+v", state.DecodePlayer(&mem))
	}
	p := state.DecodePlayer(&mem)
	if p.MapID != 0x28 {
		t.Fatalf("postcondition: expected Oak's lab (0x28), got %#04x at (%d,%d)", p.MapID, p.X, p.Y)
	}
	if party := state.DecodeParty(&mem); party.Count > 0 {
		t.Logf("lead after the balls: level %d", party.Mons[0].Level)
	}

	// A second call must be a no-op success and still leave exactly 5
	// balls: the script's IsItemInBag guard means a second talk gives
	// nothing, so a skill that treats that as failure would wedge any run
	// that resumes from a checkpoint.
	if err := skill.GetPokeBalls(m, romData, policy); err != nil {
		t.Fatalf("second GetPokeBalls: %v", err)
	}
	state.Snapshot(m, &mem)
	if n := pokeballCount(&mem); n != 5 {
		t.Fatalf("after second call: bag holds %d POKE_BALL, want 5: %+v", n, state.DecodeInventory(&mem).Items)
	}
}

// TestPostPokeballsFixture is the S6-1 fixture check: loading post_pokeballs
// must hand back the state captured after the balls talk — in Oak's lab,
// controllable, with the Route 22 battle event and the pokeballs-from-Oak
// flag set and 5x POKE_BALL in the bag. The build itself is the full journey
// (starter -> OaksParcel -> GetPokeBalls), so the test also proves the
// builder runs end to end. It is the standing state S6-3 onward loads
// instead of replaying the errand and the Route 22 battle.
func TestPostPokeballsFixture(t *testing.T) {
	m := fixture.Load(t, "post_pokeballs")

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, state.EventBeatRoute22Rival1stBattle) {
		t.Fatalf("fixture post_pokeballs: %s not set", state.EventBeatRoute22Rival1stBattle)
	}
	if !state.HasEvent(&mem, state.EventGotPokeballsFromOak) {
		t.Fatalf("fixture post_pokeballs: %s not set", state.EventGotPokeballsFromOak)
	}
	if n := pokeballCount(&mem); n != 5 {
		t.Fatalf("fixture post_pokeballs: bag holds %d POKE_BALL, want 5: %+v", n, state.DecodeInventory(&mem).Items)
	}
	if !state.Controllable(&mem) {
		t.Fatalf("fixture post_pokeballs: player not controllable: %+v", state.DecodePlayer(&mem))
	}
	p := state.DecodePlayer(&mem)
	if p.MapID != 0x28 {
		t.Fatalf("fixture post_pokeballs: expected Oak's lab (0x28), got %#04x at (%d,%d)", p.MapID, p.X, p.Y)
	}
}
