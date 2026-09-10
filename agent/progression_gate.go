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
