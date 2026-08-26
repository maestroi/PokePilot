package skill_test

import (
	"testing"
	"time"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestGetParcel is S5b-2: from the post-starter state, collect Oak's parcel
// from the Viridian Mart and return with the parcel flag set and the parcel
// in the bag. It does not deliver the parcel to Oak; that is the next task,
// so EVENT_GOT_POKEDEX is left unset.
func TestGetParcel(t *testing.T) {
	m := fixture.Load(t, "post_starter")
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)

	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.HasEvent(&mem, state.EventGotOaksParcel) {
		t.Fatal("precondition: parcel flag already set in post_starter fixture")
	}
	for _, it := range state.DecodeInventory(&mem).Items {
		if it.ID == skill.ItemOaksParcel {
			t.Fatalf("precondition: parcel already in bag: %+v", state.DecodeInventory(&mem).Items)
		}
	}

	start := time.Now()
	if err := skill.GetParcel(m, romData, policy); err != nil {
		t.Fatalf("GetParcel: %v", err)
	}
	t.Logf("GetParcel took %v", time.Since(start))

	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, state.EventGotOaksParcel) {
		t.Fatal("postcondition: parcel flag not set")
	}
	bag := state.DecodeInventory(&mem)
	parcel := false
	for _, it := range bag.Items {
		if it.ID == skill.ItemOaksParcel {
			parcel = true
			if it.Quantity != 1 {
				t.Fatalf("postcondition: parcel quantity %d, want 1", it.Quantity)
			}
		}
	}
	if !parcel {
		t.Fatalf("postcondition: no OAK's PARCEL in bag: %+v", bag.Items)
	}
	if !state.Controllable(&mem) {
		t.Fatalf("postcondition: player not controllable: %+v", state.DecodePlayer(&mem))
	}
	p := state.DecodePlayer(&mem)
	if p.MapID != 0x2A || p.X != 2 || p.Y != 5 {
		t.Fatalf("postcondition: expected (2,5) on map 0x2a, got (%d,%d) on %#04x", p.X, p.Y, p.MapID)
	}
}

// TestOaksParcel is S5b-3a: from the post-starter state, collect Oak's parcel
// from the Viridian Mart and deliver it to Professor Oak in his lab, letting
// the hand-over chain run to completion.
//
// The task text expected the hand-over to satisfy three postconditions:
// (1) EVENT_GOT_POKEDEX set, (2) EVENT_GOT_POKEBALLS_FROM_OAK set,
// (3) 5x POKE_BALL in the bag. (2) and (3) were a wrong assumption,
// verified against the decomp and measured on the real ROM: the only code
// that sets EVENT_GOT_POKEBALLS_FROM_OAK or gives the 5 balls is
// .give_poke_balls (pokered/scripts/OaksLab.asm:1022-1029), and that
// branch is reached only when EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE is set
// (OaksLab.asm:988-989) - an event with exactly one setter in the tree,
// Route22.asm:167, the after-battle script of the Route 22 rival battle,
// a later story beat this task does not play. The hand-over chain instead
// ends with the rival spawned on Route 22 (OaksLab.asm:628-649).
//
// The test therefore hard-asserts (1) plus the chain's terminal state
// (parcel consumed, player back in the lab, controllable), and pins (2)
// and (3) as the measured absence. If a ROM or script change ever makes
// the chain hand over the balls, those pins fail and must be re-derived,
// not silently deleted.
func TestOaksParcel(t *testing.T) {
	m := fixture.Load(t, "post_starter")
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)

	// Preconditions: the post-starter state has none of the hand-over results
	// yet, so the test proves OaksParcel produces each one.
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.HasEvent(&mem, state.EventGotPokedex) {
		t.Fatal("precondition: Pokedex flag already set in post_starter fixture")
	}
	if state.HasEvent(&mem, state.EventGotPokeballsFromOak) {
		t.Fatal("precondition: pokeballs-from-Oak flag already set in post_starter fixture")
	}
	for _, it := range state.DecodeInventory(&mem).Items {
		if it.ID == skill.ItemOaksParcel {
			t.Fatalf("precondition: parcel already in bag: %+v", state.DecodeInventory(&mem).Items)
		}
		if it.ID == skill.ItemPokeBall {
			t.Fatalf("precondition: pokeballs already in bag: %+v", state.DecodeInventory(&mem).Items)
		}
	}

	start := time.Now()
	if err := skill.OaksParcel(m, romData, policy); err != nil {
		t.Fatalf("OaksParcel: %v", err)
	}
	t.Logf("OaksParcel took %v", time.Since(start))

	state.Snapshot(m, &mem)

	// (1) The hand-over chain runs to completion: Oak hands over the Pokedex.
	// This is the one postcondition the parcel-delivery chain actually
	// satisfies, so a failure here is a real bug in the flow.
	if !state.HasEvent(&mem, state.EventGotPokedex) {
		t.Fatalf("postcondition: %s not set", state.EventGotPokedex)
	}

	// The parcel is consumed by the hand-over and the player stands back in
	// the lab, controllable. wJoyIgnore is non-zero (0xF0) while the chain's
	// Pokedex-giving script runs and is cleared only by the chain's final
	// script (RIVAL_LEAVES, OaksLab.asm:644-645), so Controllable together
	// with the Pokedex event proves the chain ran to its terminus, not just
	// partway through it.
	for _, it := range state.DecodeInventory(&mem).Items {
		if it.ID == skill.ItemOaksParcel {
			t.Fatalf("postcondition: parcel still in bag after delivery: %+v", state.DecodeInventory(&mem).Items)
		}
	}
	if !state.Controllable(&mem) {
		t.Fatalf("postcondition: player not controllable: %+v", state.DecodePlayer(&mem))
	}
	p := state.DecodePlayer(&mem)
	if p.MapID != 0x28 {
		t.Fatalf("postcondition: expected map 0x28 (Oak's lab), got %#04x", p.MapID)
	}

	// (2) and (3), pinned as measured ROM behavior: the hand-over chain
	// does NOT set EVENT_GOT_POKEBALLS_FROM_OAK and does NOT give 5x
	// POKE_BALL. See the function comment for the decomp evidence
	// (OaksLab.asm:988-989, 1022-1029; Route22.asm:167). The pins fail if
	// the chain or the ROM ever changes so that it does - that is the
	// point: they keep the wrong premise from silently re-entering the
	// plan, and a failure means re-derive against the new decomp, not
	// delete.
	gotPokeballsFlag := state.HasEvent(&mem, state.EventGotPokeballsFromOak)
	pokeballs := 0
	for _, it := range state.DecodeInventory(&mem).Items {
		if it.ID == skill.ItemPokeBall {
			pokeballs += int(it.Quantity)
		}
	}
	if gotPokeballsFlag {
		t.Fatalf("ROM behavior changed: hand-over chain set %s - the task's premise no longer fails; re-derive (2)/(3) against the new decomp before deleting this pin",
			state.EventGotPokeballsFromOak)
	}
	if pokeballs != 0 {
		t.Fatalf("ROM behavior changed: hand-over chain gave %d POKE_BALL(s) - re-derive (2)/(3) against the new decomp before deleting this pin", pokeballs)
	}
	t.Logf("after delivery: %s=%v, pokeballs in bag=%d (both absent by ROM design; the 5 balls need the Route 22 rival battle first - Route22.asm:167)",
		state.EventGotPokeballsFromOak, gotPokeballsFlag, pokeballs)
}
