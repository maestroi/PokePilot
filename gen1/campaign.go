package gen1

import "github.com/maestroi/pokepilot/game"

// MiddleCampaignStages is the cartridge-neutral Kanto spine after HM01 and
// Misty's Cascade Badge. Concrete adapters own how each fact is observed and
// may insert version-specific interruptions between these durable milestones.
func MiddleCampaignStages() []game.ProgressID {
	return []game.ProgressID{
		ProgressThunderBadge,
		ProgressPostSurgeLavenderReached,
		ProgressPostSurgeCeladonReady,
		ProgressRainbowBadge,
	}
}

// FirstIncomplete returns the first semantic campaign fact that has not yet
// been positively observed. It contains no map, event, item, or game-specific
// assumptions.
func FirstIncomplete(state game.ProgressState, stages []game.ProgressID) (game.ProgressID, bool) {
	for _, id := range stages {
		if !state.Has(id) {
			return id, true
		}
	}
	return "", false
}
