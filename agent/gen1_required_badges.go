package agent

import "github.com/maestroi/pokepilot/red/state"

// gen1CascadeBadgeObjectives turns Cascade from an implicit Cut precondition
// into an explicit Kanto story handoff. The transaction is version-neutral:
// after HM01 is owned, route to Cerulean Gym until the semantic Cascade badge
// is observed. The concrete profile owns the badge bit and map observation.
func gen1CascadeBadgeObjectives(obs Observation) []Objective {
	if !obs.Story.Has(redProgressHM01Acquired) || hasBadge(obs, state.BadgeCascade) {
		return nil
	}
	return []Objective{{
		Kind:  KindGoTo,
		Place: "cerulean gym",
		Note:  "(HM01 is owned but Cut is not legal until Misty is defeated; return to Cerulean Gym and earn the Cascade Badge before the Surge leg)",
	}}
}
