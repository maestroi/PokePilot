package gen1

import "github.com/maestroi/pokepilot/game"

// EarlyCampaignStages is the shared Kanto spine after the starter/opening.
// Version-specific interruptions (for example Yellow's Jessie/James exit battle)
// are inserted by the concrete adapter between these durable milestones.
func EarlyCampaignStages() []game.ProgressID {
	return []game.ProgressID{
		ProgressPokedexAcquired,
		ProgressBoulderBadge,
		ProgressMtMoonFossilAcquired,
		ProgressSSTicketAcquired,
		ProgressHM01Acquired,
	}
}

// LeagueApproachStages is the shared Kanto endgame order between the eighth
// badge and the first Elite Four room. Concrete games own how each fact is
// executed and observed.
func LeagueApproachStages() []game.ProgressID {
	return []game.ProgressID{
		ProgressRoute22RivalResolved,
		ProgressRoute23BadgeChecks,
		ProgressVictoryRoadCleared,
		ProgressIndigoPlateauReady,
	}
}

// LeagueStages is the shared Gen-I Elite Four/Champion/Hall of Fame order.
// Returning a fresh slice prevents callers from mutating global campaign
// metadata.
func LeagueStages() []game.ProgressID {
	return []game.ProgressID{
		ProgressLeagueChallengeStarted,
		ProgressLeagueLoreleiDefeated,
		ProgressLeagueBrunoDefeated,
		ProgressLeagueAgathaDefeated,
		ProgressLeagueLanceDefeated,
		ProgressLeagueChampionDefeated,
		ProgressMainStoryComplete,
	}
}

// FirstIncomplete returns the first durable semantic fact not yet complete.
// It deliberately knows nothing about maps, battles, items, or version-specific
// scripts; those remain adapter-owned.
func FirstIncomplete(state game.ProgressState, stages []game.ProgressID) (game.ProgressID, bool) {
	for _, id := range stages {
		if !state.Has(id) {
			return id, true
		}
	}
	return "", false
}
