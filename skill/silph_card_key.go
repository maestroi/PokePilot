package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	silphCo1FMap uint8 = 0xb5
	silphCo5FMap uint8 = 0xd2

	silphCardKeyItem uint8 = 0x30
	silphCardKeyX    uint8 = 21
	silphCardKeyY    uint8 = 16

	// (26,1) is the open landing immediately below 5F's stair warp at
	// (26,0), which connects to 4F. Using a known stair landing as the
	// cross-map target keeps this stage on ordinary building topology; once
	// there, Pickup chooses the actually reachable side of the Card Key ball
	// from the live grid rather than assuming a fixed final approach tile.
	silphCardKeyLandingX uint8 = 26
	silphCardKeyLandingY uint8 = 1

	silphCardKeyTravelBattles = 120
)

// SilphCardKeyOwned is the positive postcondition for the second #34 phase.
// The semantic decoder derives this from the current bag, so checkpoint resume
// never depends on run history or on remembering that the pickup text played.
func SilphCardKeyOwned(mem *state.Mem) bool {
	return state.DecodeStoryFacts(mem, state.DecodeInventory(mem)).CardKeyOwned
}

// SilphCardKeyReady makes the handoff from the Saffron-gate phase explicit.
// The Card Key objective must not attempt to route through a closed city gate.
func SilphCardKeyReady(mem *state.Mem) bool {
	return state.DecodeStoryFacts(mem, state.DecodeInventory(mem)).SaffronGateOpen
}

// AcquireSilphCardKey enters Silph Co, follows the ordinary stair topology to
// 5F, and picks up the Card Key. The operation is deliberately resumable:
// owning the key is an immediate idempotent success, and a checkpoint anywhere
// before pickup simply re-plans from the current live map on the next call.
//
// TravelFlee owns incidental encounters on the way to 5F; Pickup owns the
// final live-grid approach, bag-capacity preflight, interaction, and exact bag
// count increase. No elevator menu or teleport-pad button script is required
// for this phase.
func AcquireSilphCardKey(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: AcquireSilphCardKey: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if SilphCardKeyOwned(&mem) {
		return nil
	}
	if !SilphCardKeyReady(&mem) {
		return fmt.Errorf("skill: AcquireSilphCardKey: Saffron gate is not open")
	}

	landing := Destination{Map: silphCo5FMap, X: silphCardKeyLandingX, Y: silphCardKeyLandingY}
	if _, err := TravelFlee(m, romData, landing, policy, silphCardKeyTravelBattles); err != nil {
		return fmt.Errorf("skill: AcquireSilphCardKey: reach Silph Co 5F stair landing: %w", err)
	}
	state.Snapshot(m, &mem)
	if mem.U8(sym.CurMap) != silphCo5FMap {
		return fmt.Errorf("skill: AcquireSilphCardKey: navigation ended on map %#04x, want Silph Co 5F %#04x", mem.U8(sym.CurMap), silphCo5FMap)
	}

	if err := Pickup(m, romData, silphCardKeyX, silphCardKeyY, silphCardKeyItem, policy); err != nil {
		return fmt.Errorf("skill: AcquireSilphCardKey: collect Card Key: %w", err)
	}
	state.Snapshot(m, &mem)
	if !SilphCardKeyOwned(&mem) {
		return fmt.Errorf("skill: AcquireSilphCardKey: pickup completed without card_key_owned semantic postcondition")
	}
	return nil
}
