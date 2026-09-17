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

// SabrinaProgression is the resumable story transaction for earning the Marsh
// Badge after Silph Co is cleared. Travel stops just inside the gym entrance;
// Gym then owns the same-map teleporter maze, trainer interruptions, Sabrina,
// and the badge postcondition. Keeping the entrance leg separate prevents the
// ordinary world router from trying to solve the teleporter maze as walkable
// geometry before Gym's dedicated warp-maze motor takes over.
func SabrinaProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: SabrinaProgression: nil policy")
	}
	if m.Peek8(sym.ObtainedBadges)&(1<<uint8(state.BadgeMarsh)) != 0 {
		return nil
	}
	if m.Peek8(sym.CurMap) != saffronGymMap {
		entry := Destination{Map: saffronGymMap, X: 9, Y: 16}
		travel, err := Travel(m, romData, entry, policy, 30)
		if err != nil {
			return fmt.Errorf("skill: SabrinaProgression: reach Saffron Gym: %w", err)
		}
		if travel.BlackedOut {
			return fmt.Errorf("skill: SabrinaProgression: %w reaching Saffron Gym", ErrBlackedOut)
		}
	}
	outcome, err := Gym(m, romData, policy)
	if err != nil {
		return fmt.Errorf("skill: SabrinaProgression: %w", err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: SabrinaProgression: Sabrina battle ended with result %d", outcome)
	}
	return nil
}
