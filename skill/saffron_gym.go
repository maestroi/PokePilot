package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	saffronGymMap           uint8 = 0xB2
	saffronPokemonCenterMap uint8 = 0xB6
)

var sabrinaGym = GymInfo{
	Map:     saffronGymMap,
	Place:   "saffron gym",
	LeaderX: 9,
	LeaderY: 8,
	Badge:   state.BadgeMarsh,
	Leader:  "SABRINA",
}

func init() {
	// Saffron Gym is intentionally not a city alias: the gym entrance remains
	// blocked by Team Rocket until the Silph rescue is complete, and the agent
	// progression gate owns when this journey becomes legal. Once inside, Gym
	// owns the warp maze, trainer interruptions, Sabrina, and the badge check.
	places["saffron gym"] = Destination{Map: saffronGymMap, X: 9, Y: 9}
	places["saffron pokemon center"] = Destination{Map: saffronPokemonCenterMap, X: 3, Y: 3}
	gyms[saffronGymMap] = sabrinaGym
}

// SaffronProgression is the ordered story executor for the Marsh Badge. The
// generic gym challenge remains available while standing in the Gym, but the
// main progression chain must be able to name and execute Sabrina from any
// resumable post-Silph state instead of depending on a local catalog offer.
func SaffronProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: SaffronProgression: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeProgress(&mem).Has(state.BadgeMarsh) {
		return nil
	}
	facts := state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
	if !facts.SilphRescueComplete {
		return fmt.Errorf("skill: SaffronProgression: Silph rescue is not complete")
	}

	if m.Peek8(sym.CurMap) != saffronGymMap {
		dest, ok := Place("saffron gym")
		if !ok {
			return fmt.Errorf("skill: SaffronProgression: saffron gym place is not registered")
		}
		res, err := TravelFlee(m, romData, dest, policy, 30)
		if err != nil {
			return fmt.Errorf("skill: SaffronProgression: reach Saffron Gym: %w", err)
		}
		if res.BlackedOut {
			return fmt.Errorf("skill: SaffronProgression: %w reaching Saffron Gym", ErrBlackedOut)
		}
	}
	if got := m.Peek8(sym.CurMap); got != saffronGymMap {
		return fmt.Errorf("skill: SaffronProgression: reached map %#04x, want Saffron Gym %#04x", got, saffronGymMap)
	}

	outcome, err := Gym(m, romData, policy)
	if err != nil {
		return fmt.Errorf("skill: SaffronProgression: %w", err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: SaffronProgression: Sabrina battle outcome %v, want won", outcome)
	}

	state.Snapshot(m, &mem)
	if !state.DecodeProgress(&mem).Has(state.BadgeMarsh) {
		return fmt.Errorf("skill: SaffronProgression: Sabrina win returned without Marsh Badge")
	}
	return nil
}
