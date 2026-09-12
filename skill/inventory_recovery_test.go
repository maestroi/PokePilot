package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestEnsureProgressionPokeBallsRestocksAtCurrentMart pins the recovery path
// that was missing from Surge progression: an empty bag at a usable mart is a
// deterministic inventory problem, not a reason to bounce back to the LLM.
func TestEnsureProgressionPokeBallsRestocksAtCurrentMart(t *testing.T) {
	m := fixture.Load(t, "viridian_mart")
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)

	var before state.Mem
	state.Snapshot(m, &before)
	if got := countItem(&before, skill.ItemPokeBall); got != 0 {
		t.Fatalf("fixture precondition: POKE BALL = %d, want 0", got)
	}

	got, err := skill.EnsureProgressionPokeBalls(m, romData, policy)
	if err != nil {
		t.Fatalf("EnsureProgressionPokeBalls: %v", err)
	}
	if got != 5 {
		t.Fatalf("EnsureProgressionPokeBalls returned %d, want 5", got)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if balls := countItem(&after, skill.ItemPokeBall); balls != 5 {
		t.Fatalf("bag: POKE BALL = %d, want 5", balls)
	}
	if !state.Controllable(&after) {
		t.Fatal("postcondition: player is not controllable after inventory recovery")
	}
}
