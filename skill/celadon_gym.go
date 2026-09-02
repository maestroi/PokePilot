package skill

import "github.com/maestroi/pokepilot/red/state"

const (
	celadonCityMap uint8 = 0x06
	celadonGymMap  uint8 = 0x86
)

// Celadon is like Vermilion at the objective boundary: the meaningful gym
// challenge begins outside the building because the approach is Cut-gated.
// Unlike Surge, Erika has no internal puzzle, so the ordinary Gym Travel to
// Place("celadon gym") can own both the exterior Cut recovery and the trainer
// battles on the way to the leader.
func init() {
	erika := GymInfo{
		Map:     celadonGymMap,
		Place:   "celadon gym",
		LeaderX: 4,
		LeaderY: 3,
		Badge:   state.BadgeRainbow,
		Leader:  "ERIKA",
	}
	gyms[celadonCityMap] = erika // exterior Cut prerequisite
	gyms[celadonGymMap] = erika  // already inside
}
