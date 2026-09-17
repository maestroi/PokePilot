package agent

import (
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// redCascadeBadgeObjectives closes the story-order gap before Lt. Surge. HM01
// can be acquired while Misty is still unbeaten, but Cut cannot be used until
// the Cascade Badge is owned. Surface that missing badge as an explicit
// progression step instead of leaving the planner with an implicit capability
// failure at Vermilion.
func redCascadeBadgeObjectives(obs Observation) []Objective {
	if !obs.Story.Has(redProgressHM01Acquired) || hasBadge(obs, state.BadgeCascade) {
		return nil
	}

	gym, ok := skill.Place("cerulean gym")
	if !ok {
		return nil
	}
	if obs.Map == gym.Map {
		return []Objective{{
			Kind:  KindGym,
			Place: "cerulean gym",
			Note:  "(HM01 is owned but Cut is still badge-locked; defeat Misty and verify the Cascade Badge before continuing to Lt. Surge)",
		}}
	}
	return []Objective{{
		Kind:  KindGoTo,
		Place: "cerulean gym",
		Note:  "(HM01 is owned but Cut is still badge-locked; return to Cerulean Gym and defeat Misty for the Cascade Badge before continuing to Lt. Surge)",
	}}
}

func redRainbowBadgeComplete(obs Observation) bool {
	return obs.Story.Has(redProgressRainbowBadge) || hasBadge(obs, state.BadgeRainbow)
}

// redEnforceRainbowBadge keeps every post-Celadon story beat behind Erika.
// Earlier code offered Silph Scope/Poke Flute on Thunder alone, which allowed a
// run to reach Fuchsia and even later story states while Rainbow remained
// missing. Filtering at the adapter progression seam also repairs resumed
// states produced by the old ordering: they are sent back through Erika before
// any downstream story objective is exposed again.
func redEnforceRainbowBadge(obs Observation, objectives []Objective) []Objective {
	if redRainbowBadgeComplete(obs) {
		return objectives
	}
	filtered := make([]Objective, 0, len(objectives))
	for _, objective := range objectives {
		if redProgressionRequiresRainbow(objective) {
			continue
		}
		filtered = append(filtered, objective)
	}
	return filtered
}

func redProgressionRequiresRainbow(objective Objective) bool {
	if objective.Kind != KindProgress {
		return false
	}
	switch objective.Progress {
	case redProgressSilphScopeAcquired,
		redProgressPokeFluteAcquired,
		redProgressFuchsiaProgressionComplete,
		ProgressSaffronGateOpen,
		ProgressCardKeyOwned,
		redProgressSilphRescueComplete,
		ProgressSecretKeyOwned,
		redProgressVolcanoBadge,
		redProgressEarthBadge,
		ProgressRoute22RivalResolved,
		ProgressRoute23BadgeChecks,
		redProgressVictoryRoadCleared,
		redProgressIndigoPlateauReady,
		ProgressLeagueChallengeStarted,
		redProgressLeagueLoreleiDefeated,
		redProgressLeagueBrunoDefeated,
		redProgressLeagueAgathaDefeated,
		redProgressLeagueLanceDefeated,
		ProgressLeagueChampionDefeated,
		ProgressMainStoryComplete:
		return true
	default:
		return false
	}
}
