package skill

import "github.com/maestroi/pokepilot/red/state"

const (
	saffronGymMap           uint8 = 0xB2
	saffronPokemonCenterMap uint8 = 0xB6
)

var sabrinaGym = GymInfo{
	Map:     saffronGymMap,
	Place:   "saffron gym",
	LeaderX: 9,
	LeaderY: 8,
	Badge:   state.BadgeMarsh,
	Leader:  "SABRINA",
}

func init() {
	// Saffron Gym is intentionally not a city alias: the gym entrance remains
	// blocked by Team Rocket until the Silph rescue is complete, and the agent
	// progression gate owns when this journey becomes legal. Once inside, Gym
	// owns the warp maze, trainer interruptions, Sabrina, and the badge check.
	places["saffron gym"] = Destination{Map: saffronGymMap, X: 9, Y: 9}
	places["saffron pokemon center"] = Destination{Map: saffronPokemonCenterMap, X: 3, Y: 3}
	gyms[saffronGymMap] = sabrinaGym
}
