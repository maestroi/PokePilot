package agent

import "github.com/maestroi/pokepilot/red/state"

// redCascadeBadgeObjectives is retained as a compatibility wrapper while Red
// call sites/tests migrate to the shared Gen-I handoff.
func redCascadeBadgeObjectives(obs Observation) []Objective {
	return gen1CascadeBadgeObjectives(obs)
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
