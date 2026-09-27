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

// RocketTowerStages is the shared Kanto story bridge from Celadon's Rocket
// Hideout through Mr. Fuji's Poké Flute handoff. Concrete cartridges may
// insert scripted battles (Yellow's Jessie/James encounters), but the durable
// item milestones are common.
func RocketTowerStages() []game.ProgressID {
	return []game.ProgressID{
		ProgressSilphScopeAcquired,
		ProgressPokeFluteAcquired,
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
