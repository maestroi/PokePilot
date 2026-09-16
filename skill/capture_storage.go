package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// EnsureCaptureStorage prepares Gen I storage for an actual capture without
// forcing a free party slot. When the party already has six Pokemon, Red sends
// a successful wild/static/Safari catch directly to the active PC box. The only
// storage precondition in that case is that the active box has room; if it is
// full, switch to another non-full box while leaving the progression-critical
// party untouched.
func EnsureCaptureStorage(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: capture storage: nil move policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	box := state.DecodeBox(&mem)
	if !captureNeedsBoxSwitch(party.Count, box.Count) {
		return nil
	}
	return SwitchToNextNonFullBox(m, romData, policy)
}

func captureNeedsBoxSwitch(partyCount, boxCount uint8) bool {
	return int(partyCount) >= gen1PartyCapacity && int(boxCount) >= gen1BoxCapacity
}
