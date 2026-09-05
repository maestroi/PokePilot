package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// TeachTMHMToSlot executes a previously planner-visible recipient choice.
// DecideTMHM remains the authority for compatibility and replacement policy;
// this wrapper only makes sure the decision has not silently changed between
// offering the objective and executing it.
func TeachTMHMToSlot(m *emu.Emu, item uint8, required bool, wantSlot int) (TMHMResult, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	decision, err := DecideTMHM(m.ROM(), state.DecodeParty(&mem), item, required)
	if err != nil {
		return TMHMResult{}, fmt.Errorf("skill: TeachTMHMToSlot: %w", err)
	}
	if decision.PartySlot != wantSlot {
		return TMHMResult{}, fmt.Errorf(
			"skill: TeachTMHMToSlot: planned party slot %d is stale; current best compatible slot is %d",
			wantSlot, decision.PartySlot,
		)
	}
	return TeachTMHM(m, item, required)
}
