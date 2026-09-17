package agent

import "github.com/maestroi/pokepilot/red/state"

// redBadgeOrderedProgression keeps required badges on the explicit story path.
// The generic challenge catalog can surface gyms opportunistically, but Red's
// critical path must not be able to advance past a badge that unlocks the next
// field/story leg.
func redBadgeOrderedProgression(obs Observation, objectives []Objective) []Objective {
	// Misty's Cascade Badge is the field-use prerequisite for Cut. Once Bill's
	// S.S. Ticket beat is complete, make Cerulean Gym the next story obligation
	// before the S.S. Anne / Surge chain can continue indefinitely without it.
	if obs.Story.Has(redProgressSSTicketAcquired) && !hasBadge(obs, state.BadgeCascade) {
		kind := KindGoTo
		if obs.Map == ceruleanGymMap {
			kind = KindGym
		}
		return []Objective{{
			Kind:  kind,
			Place: "cerulean gym",
			Note:  "(defeat Misty and verify the Cascade Badge before continuing the Cut/Lt. Surge story leg)",
		}}
	}

	// Erika is part of the ordered post-Surge path. While Rainbow is missing,
	// keep the travel/checkpoint beats that lead to Celadon plus the badge
	// objective itself, but suppress Rocket Hideout and every later story beat.
	if hasBadge(obs, state.BadgeThunder) && !hasBadge(obs, state.BadgeRainbow) {
		filtered := make([]Objective, 0, len(objectives))
		for _, objective := range objectives {
			if objective.Kind != KindProgress {
				filtered = append(filtered, objective)
				continue
			}
			switch objective.Progress {
			case redProgressPostSurgeLavenderReached,
				redProgressPostSurgeCeladonReady,
				redProgressRainbowBadge:
				filtered = append(filtered, objective)
			}
		}
		return filtered
	}

	return objectives
}
