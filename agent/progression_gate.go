package agent

import "github.com/maestroi/pokepilot/red/state"

const (
	saffronCityMap uint8 = 0x0a
	route2Map      uint8 = 0x0d
	route3Map      uint8 = 0x0e
	viridianGymMap uint8 = 0x2d
	cinnabarGymMap uint8 = 0xa6
)

// journeyProgressionBlocked reports a journey the game itself cannot currently
// complete. These are scripted exits/doors, so the destination is not merely
// risky: the walk cannot finish until the live prerequisite is satisfied.
// Keep such objectives off the planner menu until the prerequisite is visible
// in Observation.
//
// Early gates use the generic badge/event fields that predate StoryFacts.
// Later-game gates use Observation.Story so Offer never has to know which Red
// event bit, status bit, or item id proves the semantic condition.
func journeyProgressionBlocked(obs Observation, destinationMap uint8) bool {
	switch destinationMap {
	case route3Map:
		// PewterCityDefaultScript force-stops the east exit until Brock has
		// been beaten.
		return !hasBadge(obs, state.BadgeBoulder)
	case route2Map:
		// The Viridian guard moves only once Oak's parcel has been delivered;
		// receiving the Pokédex is the observable completion of that errand.
		return !observedEvent(obs, state.EventGotPokedex.String())
	case saffronCityMap:
		// All four Saffron guards share the persistent drink-given status bit.
		return !obs.Story.SaffronGateOpen
	case cinnabarGymMap:
		// The Gym door consumes no key, but remains locked until the Secret
		// Key is present in the bag.
		return !obs.Story.SecretKeyOwned
	case viridianGymMap:
		// Viridian City's script sets the persistent gym-open event once the
		// story prerequisite is satisfied.
		return !obs.Story.ViridianGymOpen
	}
	return false
}
