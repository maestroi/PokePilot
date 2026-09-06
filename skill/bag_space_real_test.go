package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestEnsureBagFreeSlotsReal is an opt-in ROM-backed smoke test for the real
// START -> ITEM -> TOSS -> quantity -> confirmation flow. It deliberately
// uses a normal early-game fixture rather than mutating RAM into a synthetic
// 20-stack bag: when the fixture already has a free slot the skill must be a
// no-op. Full-bag toss behavior is covered by the deterministic policy tests
// and can be exercised by a prepared qualification state without shipping ROM
// or save-state data in the repository.
func TestEnsureBagFreeSlotsReal(t *testing.T) {
	if testing.Short() {
		t.Skip("emulator journey test")
	}
	m := fixture.Load(t, "viridian_mart")
	var before state.Mem
	state.Snapshot(m, &before)
	beforeInv := state.DecodeInventory(&before)

	if err := skill.EnsureBagFreeSlots(m, 1); err != nil {
		t.Fatalf("EnsureBagFreeSlots(1): %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	afterInv := state.DecodeInventory(&after)
	if len(afterInv.Items) != len(beforeInv.Items) {
		t.Fatalf("bag changed despite already having room: %d -> %d stacks", len(beforeInv.Items), len(afterInv.Items))
	}
}
