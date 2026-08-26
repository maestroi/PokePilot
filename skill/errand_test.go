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
// It verifies the one postcondition the delivery chain actually satisfies
// (EVENT_GOT_POKEDEX set, parcel consumed, player back in the lab and
// controllable). It also checks the two postconditions the task expected from
// the hand-over — EVENT_GOT_POKEBALLS_FROM_OAK set and 5x POKE_BALL in the
// bag — and, because the ROM shows those are gated on a much-later Route 22
// battle (see the in-body note), it fails with a "WRONG ASSUMPTION" diagnostic
// carrying the evidence. It is intentionally not weakened to pass.
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
	// the lab, controllable.
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

	// (2) and (3) — the task's premise that delivering the parcel makes Oak
	// hand over 5x POKE_BALL (setting EVENT_GOT_POKEBALLS_FROM_OAK).
	//
	// The ROM does not do this. The only code that sets
	// EVENT_GOT_POKEBALLS_FROM_OAK or gives the 5 balls is .give_poke_balls
	// (pokered/scripts/OaksLab.asm:1022-1025), and it is reached only when
	// EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE is set (OaksLab.asm:988-989). That
	// event is set in exactly one place, Route22.asm:167 — a Route 22 rival
	// battle many towns later in the game. The parcel-delivery chain
	// (RIVAL_ARRIVES -> OAK_GIVES_POKEDEX -> RIVAL_LEAVES) never sets it, so
	// neither postcondition is reachable from the post-starter + parcel state.
	//
	// If a future ROM change did make them reachable, this test passes; as
	// it stands it documents the wrong assumption by failing with evidence.
	gotPokeballsFlag := state.HasEvent(&mem, state.EventGotPokeballsFromOak)
	pokeballs := 0
	for _, it := range state.DecodeInventory(&mem).Items {
		if it.ID == skill.ItemPokeBall {
			pokeballs += int(it.Quantity)
		}
	}
	if !gotPokeballsFlag || pokeballs != 5 {
		t.Fatalf("WRONG ASSUMPTION (S5b-3a): postconditions (2) %s and (3) 5x POKE_BALL are unreachable from post_starter+parcel. Actual after delivery: %s=%v, pokeballs in bag=%d, bag=%+v. They require EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE (OaksLab.asm:988-989), set only at Route22.asm:167 (a Route 22 battle far later in the game). The parcel chain only gives the Pokedex (postcondition 1, which passed).",
			state.EventGotPokeballsFromOak, state.EventGotPokeballsFromOak, gotPokeballsFlag, pokeballs, state.DecodeInventory(&mem).Items)
	}
}
