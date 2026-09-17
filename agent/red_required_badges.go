package agent

import "github.com/maestroi/pokepilot/red/state"

// redCascadeBadgeObjectives turns Cascade from an implicit Cut precondition
// into an explicit story handoff. Away from Cerulean the progression provider
// supplies the journey; once there, the ordinary typed gym catalog supplies
// the challenge itself and its positive badge postcondition.
func redCascadeBadgeObjectives(obs Observation) []Objective {
	if !obs.Story.Has(redProgressHM01Acquired) || hasBadge(obs, state.BadgeCascade) {
		return nil
	}
	return []Objective{{
		Kind:  KindGoTo,
		Place: "cerulean gym",
		Note:  "(HM01 is owned but Cut is not legal until Misty is defeated; return to Cerulean Gym and earn the Cascade Badge before the Surge leg)",
	}}
}

// redRequireRainbowForPostCeladon removes later local story offers until Erika
// has been positively completed. The Red world allows some badge-order freedom,
// but the deterministic PokePilot chain must not defer Rainbow until the final
// eight-badge gate.
func redRequireRainbowForPostCeladon(obs Observation, objectives []Objective) []Objective {
	if hasBadge(obs, state.BadgeRainbow) {
		return objectives
	}
	filtered := objectives[:0]
	for _, objective := range objectives {
		if objective.Kind == KindProgress &&
			(objective.Progress == redProgressSilphScopeAcquired || objective.Progress == redProgressPokeFluteAcquired) {
			continue
		}
		filtered = append(filtered, objective)
	}
	return filtered
}
