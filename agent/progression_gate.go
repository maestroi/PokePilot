package agent

import "github.com/maestroi/pokepilot/red/state"

const route3Map uint8 = 0x0e

// journeyProgressionBlocked reports a journey the game itself cannot currently
// complete. PewterCityDefaultScript force-stops the east exit every frame until
// Brock has been beaten; Route 3 is therefore not merely risky pre-Boulder — it
// is unreachable. Keep that impossible objective off the planner menu until the
// badge is visible in the live observation. This gate is independent of trainer
// loss recovery: successful training changes combat readiness, but it cannot
// open Pewter's scripted exit.
func journeyProgressionBlocked(obs Observation, destinationMap uint8) bool {
	return destinationMap == route3Map && !hasBadge(obs, state.BadgeBoulder)
}
