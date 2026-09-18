package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	boulderPewterCityMap uint8 = 0x02
	boulderPewterGymMap  uint8 = 0x36
	boulderTravelBattles       = 80
)

// BoulderProgression makes the first badge a resumable semantic story step
// rather than leaving Route 3's gate to depend on the strategist noticing a
// local gym. It may start from any routable post-Pokedex overworld state.
func BoulderProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: BoulderProgression: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeProgress(&mem).Has(state.BadgeBoulder) {
		return nil
	}

	switch m.Peek8(sym.CurMap) {
	case boulderPewterCityMap, boulderPewterGymMap:
		// Already at the atomic gym slice.
	default:
		pewter, ok := Place("pewter city")
		if !ok {
			return fmt.Errorf("skill: BoulderProgression: pewter city place missing")
		}
		res, err := TravelFlee(m, romData, pewter, policy, boulderTravelBattles)
		if err != nil {
			return fmt.Errorf("skill: BoulderProgression: reach Pewter: %w", err)
		}
		if res.BlackedOut {
			return fmt.Errorf("skill: BoulderProgression: %w reaching Pewter", ErrBlackedOut)
		}
	}

	outcome, err := Gym(m, romData, policy)
	if err != nil {
		return fmt.Errorf("skill: BoulderProgression: Brock: %w", err)
	}
	if outcome == state.ResultLost {
		return fmt.Errorf("skill: BoulderProgression: %w against Brock", ErrTrainerBlackedOut)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: BoulderProgression: Brock battle ended with outcome %d", outcome)
	}

	state.Snapshot(m, &mem)
	if !state.DecodeProgress(&mem).Has(state.BadgeBoulder) {
		return fmt.Errorf("skill: BoulderProgression: Boulder Badge missing after Brock")
	}
	return nil
}
