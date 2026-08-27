package skill_test

import (
	"testing"
	"time"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestHeal is S5b-4: from the viridian_pokecenter fixture (the player on
// the counter approach tile), heal the party at the nurse and return with
// every mon at full HP and the player controllable, still on the approach
// tile.
//
// The fixture arrives damaged: its builder Travels across Route 1's tall
// grass, and the wild Pidgey battle the trip throws leaves the party mon at
// 13/22 HP (measured on the committed v4 state, 2026-08-27), with
// BIT_USED_POKECENTER unset, so the first-visit "Shall we heal your
// Pokemon?" box appears as well as the welcome box. The precondition below
// pins the damage: if a fixture version ever arrives at full HP, the test
// fails here rather than passing against a no-op heal.
func TestHeal(t *testing.T) {
	m := fixture.Load(t, "viridian_pokecenter")

	want, ok := skill.Place("viridian pokemon center")
	if !ok {
		t.Fatal(`Place: "viridian pokemon center" not found`)
	}

	// Preconditions: on the approach tile, controllable, party damaged.
	var mem state.Mem
	state.Snapshot(m, &mem)
	p := state.DecodePlayer(&mem)
	if p.MapID != want.Map || p.X != want.X || p.Y != want.Y {
		t.Fatalf("precondition: fixture player at (%d,%d) on %#04x, want (%d,%d) on %#04x",
			p.X, p.Y, p.MapID, want.X, want.Y, want.Map)
	}
	if !state.Controllable(&mem) {
		t.Fatalf("precondition: fixture player not controllable: %+v", p)
	}
	party := state.DecodeParty(&mem)
	if party.Count == 0 {
		t.Fatal("precondition: fixture has no party")
	}
	beforeCount := party.Count
	damaged := 0
	for i, mon := range party.Mons {
		t.Logf("before heal: mon%d species=%#04x lvl=%d HP=%d/%d status=%#04x",
			i+1, mon.Species, mon.Level, mon.HP, mon.MaxHP, mon.Status)
		if mon.HP < mon.MaxHP {
			damaged++
		}
	}
	if damaged == 0 {
		t.Fatalf("precondition: fixture party at full HP; the heal pre-assertion would be vacuous: %+v", party.Mons)
	}

	start := time.Now()
	if err := skill.Heal(m); err != nil {
		t.Fatalf("Heal: %v", err)
	}
	t.Logf("Heal took %v", time.Since(start))

	state.Snapshot(m, &mem)

	// Postcondition: every mon at full HP, the party the same size, the
	// player controllable again and still on the approach tile (the heal
	// sequence does not move the player).
	party = state.DecodeParty(&mem)
	if party.Count != beforeCount {
		t.Fatalf("postcondition: party count changed %d -> %d", beforeCount, party.Count)
	}
	for i, mon := range party.Mons {
		t.Logf("after heal: mon%d species=%#04x HP=%d/%d", i+1, mon.Species, mon.HP, mon.MaxHP)
		if mon.HP != mon.MaxHP {
			t.Fatalf("postcondition: mon %d at %d/%d after heal, want full: %+v", i+1, mon.HP, mon.MaxHP, party.Mons)
		}
	}
	if !state.Controllable(&mem) {
		t.Fatalf("postcondition: player not controllable: %+v", state.DecodePlayer(&mem))
	}
	p = state.DecodePlayer(&mem)
	if p.MapID != want.Map || p.X != want.X || p.Y != want.Y {
		t.Fatalf("postcondition: player at (%d,%d) on %#04x, want (%d,%d) on %#04x",
			p.X, p.Y, p.MapID, want.X, want.Y, want.Map)
	}
}
