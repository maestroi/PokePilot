package agent

import "github.com/maestroi/pokepilot/red/state"

// filterRedProgressionStageObjectives keeps Red-specific milestone ordering
// authoritative after generic Offer has contributed local verbs and journeys.
// Celadon exposes both a generic Gym action and a journey whose destination is
// the leader tile. Before the explicit post-Surge recovery stage is satisfied,
// either one would let the planner bypass the bounded stage sequence and fold
// gym approach/battles back into an earlier transaction.
func filterRedProgressionStageObjectives(obs Observation, out []Objective) []Objective {
	if !hasBadge(obs, state.BadgeThunder) || hasBadge(obs, state.BadgeRainbow) ||
		obs.Story.Has(redProgressPostSurgeCeladonReady) {
		return out
	}

	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if o.Place == "celadon gym" && (o.Kind == KindGym || o.Kind == KindGoTo) {
			continue
		}
		filtered = append(filtered, o)
	}
	return filtered
}
