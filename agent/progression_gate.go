package agent

import "github.com/maestroi/pokepilot/red/state"

const (
	saffronCityMap uint8 = 0x0a
	route2Map      uint8 = 0x0d
	viridianGymMap uint8 = 0x2d
	route3Map      uint8 = 0x0e
	cinnabarGymMap uint8 = 0xa6
	saffronGymMap  uint8 = 0xb2
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
	}
	return false
}
