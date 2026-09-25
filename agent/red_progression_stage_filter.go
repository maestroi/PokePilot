package agent

import "github.com/maestroi/pokepilot/red/state"

// filterRedProgressionStageObjectives keeps Red-specific milestone ordering
// authoritative after generic Offer has contributed local verbs and journeys.
// Some story gyms also appear through the generic challenge/travel catalogs, so
// those generic candidates must not bypass the explicit campaign prerequisite
// that owns their approach.
func filterRedProgressionStageObjectives(obs Observation, out []Objective) []Objective {
	blockCeladonGym := hasBadge(obs, state.BadgeThunder) &&
		!hasBadge(obs, state.BadgeRainbow) &&
		!obs.Story.Has(redProgressPostSurgeCeladonReady)

	// Cinnabar Gym's exterior door is hard-locked by the Mansion Secret Key.
	// The Silph Card Key is a separate item/fact and must never make this gym
	// selectable. Keep an already-inside checkpoint recoverable, matching the
	// route-requirement rule that gates entry rather than local actions.
	blockCinnabarGym := obs.Map != cinnabarGymMap &&
		!obs.Story.Has(ProgressSecretKeyOwned)

	if !blockCeladonGym && !blockCinnabarGym {
		return out
	}

	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if blockCeladonGym && o.Place == "celadon gym" && (o.Kind == KindGym || o.Kind == KindGoTo) {
			continue
		}
		if blockCinnabarGym && o.Place == "cinnabar gym" && (o.Kind == KindGym || o.Kind == KindGoTo) {
			continue
		}
		filtered = append(filtered, o)
	}
	return filtered
}
