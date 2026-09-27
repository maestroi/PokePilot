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

// FuchsiaStages is the shared Kanto handoff after the Poké Flute. The
// progression transaction owns Route 12 Snorlax, Koga/Soul, HM03 Surf and
// HM04 Strength as one resumable semantic milestone.
func FuchsiaStages() []game.ProgressID {
	return []game.ProgressID{
		ProgressFuchsiaProgressionComplete,
	}
}

// CinnabarStages is the shared Kanto Mansion/Gym transaction after Surf is
// available: obtain the Secret Key, then unlock Cinnabar Gym and defeat Blaine.
func CinnabarStages() []game.ProgressID {
	return []game.ProgressID{
		ProgressSecretKeyOwned,
		ProgressVolcanoBadge,
	}
}

// SaffronStages is the shared late-game story bridge from opening Saffron's
// guardhouses through Silph Co and Sabrina. Concrete cartridges may insert
// version-specific scripted battles inside Silph while retaining these durable
// semantic boundaries.
func SaffronStages() []game.ProgressID {
	return []game.ProgressID{
		ProgressSaffronGateOpen,
		ProgressCardKeyOwned,
		ProgressSilphRescueComplete,
		ProgressMarshBadge,
	}
}

// LeagueApproachStages is the shared eight-badge path from Giovanni through
// the final Route 22 rival, Route 23 badge gates, Victory Road and the healed
// Indigo Plateau lobby. It intentionally stops before the Elite Four gauntlet.
func LeagueApproachStages() []game.ProgressID {
	return []game.ProgressID{
		ProgressEarthBadge,
		ProgressRoute22RivalResolved,
		ProgressRoute23BadgeChecks,
		ProgressVictoryRoadCleared,
		ProgressIndigoPlateauReady,
	}
}

// LeagueStages is the shared no-exit Kanto League sequence from committing
// to Lorelei's room through the Champion and durable Hall-of-Fame completion.
// Concrete games own how each member/completion fact is decoded from native
// event/RAM state; the staged controller mechanics are shared Gen I.
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
