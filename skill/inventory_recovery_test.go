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
// The fixture has ¥1587, so a ten-ball reserve is unaffordable at ¥200 each;
// recovery must degrade to the largest affordable quantity (seven), not fail.
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
	if got != 7 {
		t.Fatalf("EnsureProgressionPokeBalls returned %d, want 7", got)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if balls := countItem(&after, skill.ItemPokeBall); balls != 7 {
		t.Fatalf("bag: POKE BALL = %d, want 7", balls)
	}
	if !state.Controllable(&after) {
		t.Fatal("postcondition: player is not controllable after inventory recovery")
	}
}
