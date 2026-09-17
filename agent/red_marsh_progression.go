package agent

import "github.com/maestroi/pokepilot/red/state"

// redMarshBadgeObjectives makes Sabrina an explicit story step after the
// Silph rescue. The ordinary challenge catalog only exposes a gym while the
// player is already standing inside it, which can leave a resumed run with no
// story objective between Silph Co and the Cinnabar gate. Keep the sequencing
// adapter-owned while reusing the existing GoTo and Gym executors and their
// positive postconditions.
func redMarshBadgeObjectives(obs Observation) []Objective {
	if !obs.Story.Has(redProgressSilphRescueComplete) || hasBadge(obs, state.BadgeMarsh) {
		return nil
	}

	if obs.Map == saffronGymMap {
		return []Objective{{
			Kind:  KindGym,
			Place: "saffron gym",
			Note:  "(Silph is clear; traverse Saffron Gym's warp maze, defeat Sabrina, and verify the Marsh Badge before continuing to Cinnabar)",
		}}
	}

	return []Objective{{
		Kind:  KindGoTo,
		Place: "saffron gym",
		Note:  "(Silph is clear; go to Saffron Gym next so Sabrina can be defeated for the Marsh Badge before continuing to Cinnabar)",
	}}
}
