package agent

import "github.com/maestroi/pokepilot/red/state"

const (
	lavenderTownMap uint8 = 0x04
	celadonCityMap  uint8 = 0x06
	fuchsiaCityMap  uint8 = 0x07
	saffronCityMap  uint8 = 0x0a
	route2Map       uint8 = 0x0d
	route3Map       uint8 = 0x0e
	route9Map       uint8 = 0x14
	route10Map      uint8 = 0x15
	route12Map      uint8 = 0x17
	route13Map      uint8 = 0x18
	route14Map      uint8 = 0x19
	route15Map      uint8 = 0x1a
	viridianGymMap  uint8 = 0x2d
	vermilionGymMap uint8 = 0x5c
	cinnabarGymMap  uint8 = 0xa6
	saffronGymMap   uint8 = 0xb2
)

// placeProgressionBlocked gates named waypoints that sit on the far side of
// a story obstacle within the player's own map, where journeyProgressionBlocked's
// per-map check can't help. Route 12's sleeping Snorlax is a live object rather
// than static collision, so these interaction waypoints must stay unavailable
// until the Poké Flute story fact is durable.
func placeProgressionBlocked(obs Observation, placeName string) bool {
	switch placeName {
	case "route 12 snorlax", "route 12 south of snorlax":
		return !obs.Story.Has(redProgressPokeFluteAcquired)
	}
	return false
}

func journeyProgressionBlocked(obs Observation, destinationMap uint8) bool {
	switch destinationMap {
	case route3Map:
		return !hasBadge(obs, state.BadgeBoulder)
	case route2Map:
		return !observedEvent(obs, state.EventGotPokedex.String())

	// Keep the campaign on its intended post-Cerulean critical path. Once
	// HM01 exists the ROM technically allows Red to wander toward Rock Tunnel
	// before fighting Surge, but doing so hides the still-missing third badge
	// behind long travel and lets the strategist invent later-game plans. The
	// compound Thunder-Badge objective owns recovery back to Vermilion instead.
	case route9Map, route10Map, lavenderTownMap, celadonCityMap:
		return !hasBadge(obs, state.BadgeThunder)

	// Route 12 is not an alternate early-game road to Lavender/Fuchsia. The
	// Snorlax at (10,62) blocks the corridor until Pokémon Tower yields the
	// Poké Flute. Routes 13-15 and Fuchsia are the next slice after that gate;
	// Surf/Strength are acquired in Fuchsia, not prerequisites for Route 12.
	case route12Map, route13Map, route14Map, route15Map, fuchsiaCityMap:
		return !obs.Story.Has(redProgressPokeFluteAcquired)

	case saffronCityMap:
		return !obs.Story.Has(ProgressSaffronGateOpen)
	case saffronGymMap:
		return !obs.Story.Has(redProgressSilphRescueComplete)
	case cinnabarGymMap:
		return !obs.Story.Has(ProgressSecretKeyOwned)
	case viridianGymMap:
		return !obs.Story.Has(ProgressViridianGymOpen)
	case vermilionGymMap:
		// The exterior tree is owned by the atomic/compound Surge progression,
		// not plain GoTo. Keep generic travel out of the gym from outside.
		return obs.Map != vermilionGymMap
	}
	return false
}
