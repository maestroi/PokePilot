package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// executeRedGymProgression turns a mandatory badge into a resumable story
// transaction instead of relying on the planner to happen to stand on the gym
// map. The progression chain owns when the badge is required; Travel owns the
// route and incidental battles, and Gym owns the leader battle plus the native
// badge postcondition.
func executeRedGymProgression(m *emu.Emu, romData []byte, policy skill.MovePolicy, place string) error {
	dest, ok := skill.Place(place)
	if !ok {
		return fmt.Errorf("agent: badge progression: unknown gym place %q", place)
	}
	travel, err := skill.Travel(m, romData, dest, policy, 40)
	if err != nil {
		return fmt.Errorf("agent: badge progression: reach %s: %w", place, err)
	}
	if travel.BlackedOut {
		return fmt.Errorf("agent: badge progression: %w reaching %s", skill.ErrBlackedOut, place)
	}
	outcome, err := skill.Gym(m, romData, policy)
	if err != nil {
		return fmt.Errorf("agent: badge progression: challenge %s: %w", place, err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("agent: badge progression: %s battle ended with result %d", place, outcome)
	}
	return nil
}
