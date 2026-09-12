package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const surgeProgressionTravelEngagements = 100

// SurgeProgression is the resumable handoff from HM01 to the Thunder Badge.
// It is intentionally callable from anywhere the route model can recover from:
// planner strategy should not lose sight of the third badge merely because an
// earlier bad journey wandered away from Vermilion.
func SurgeProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: SurgeProgression: nil move policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeProgress(&mem).Has(state.BadgeThunder) {
		return nil
	}

	// Repair before travel, not only at the gym door. A resumed run may be on
	// the far side of Route 9, whose legal return route itself requires Cut.
	if err := RepairUtilityFieldCapability(m, romData, policy, FieldCut); err != nil {
		return fmt.Errorf("skill: SurgeProgression: prepare Cut carrier: %w", err)
	}

	if m.Peek8(sym.CurMap) != vermilionGymMap {
		city, ok := Place("vermilion city")
		if !ok {
			return fmt.Errorf("skill: SurgeProgression: vermilion city place missing")
		}
		if _, err := TravelFlee(m, romData, city, policy, surgeProgressionTravelEngagements); err != nil {
			return fmt.Errorf("skill: SurgeProgression: return to Vermilion: %w", err)
		}
	}

	outcome, err := Gym(m, romData, policy)
	if err != nil {
		return fmt.Errorf("skill: SurgeProgression: Lt. Surge: %w", err)
	}
	if outcome == state.ResultLost {
		return fmt.Errorf("skill: SurgeProgression: %w against Lt. Surge", ErrTrainerBlackedOut)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: SurgeProgression: Lt. Surge battle ended with outcome %d", outcome)
	}

	state.Snapshot(m, &mem)
	if !state.DecodeProgress(&mem).Has(state.BadgeThunder) {
		return fmt.Errorf("skill: SurgeProgression: Thunder Badge missing after Lt. Surge")
	}
	return nil
}
