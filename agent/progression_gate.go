package agent

import "github.com/maestroi/pokepilot/red/state"

const (
	saffronCityMap  uint8 = 0x0a
	route2Map       uint8 = 0x0d
	viridianGymMap  uint8 = 0x2d
	route3Map       uint8 = 0x0e
	cinnabarGymMap  uint8 = 0xa6
	saffronGymMap   uint8 = 0xb2
	vermilionGymMap uint8 = 0x5c
)

// placeProgressionBlocked gates named waypoints that sit on the far side of
// a story obstacle within the player's own map, where journeyProgressionBlocked's
// per-map check can't help: route12Map is also the map the player is
// standing on, so it is never itself blocked. The static route graph has no
// notion of the sleeping Snorlax sprite either (see world grid decode), so
// RoutePlanner.Reachability reports both waypoints as reachable and GoTo
// discovers the obstacle only at execution time, forever failing with "no
// path" and getting re-offered every round.
// MEASURED 2026-09-11 on run-1pwifxdtjnwa52ecxjcvvmh6ch (and 9 sibling
// runs): "go to route 12 snorlax" / "go to route 12 south of snorlax" looped
// for rounds 18-22 with badges 2/8, well before the Poké Flute exists.
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
	case saffronCityMap:
		return !obs.Story.Has(ProgressSaffronGateOpen)
	case saffronGymMap:
		return !obs.Story.Has(redProgressSilphRescueComplete)
	case cinnabarGymMap:
		return !obs.Story.Has(ProgressSecretKeyOwned)
	case viridianGymMap:
		return !obs.Story.Has(ProgressViridianGymOpen)
	case vermilionGymMap:
		// The gym door is behind a Cut tree only EnterVermilionGym knows how
		// to clear (skill/cut.go); plain GoTo/Traverse has no edge onto this
		// map and stalls oscillating between Vermilion City's neighboring
		// routes (measured: run-2p2b5kf4qza0o1cv5vo5swhzxr). "beat the gym
		// leader here" (KindGym) is the only offered way in from outside.
		return obs.Map != vermilionGymMap
	}
	return false
}
