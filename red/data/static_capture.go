package data

// StaticCaptureSite describes one finite overworld encounter in Pokémon Red.
// All raw map/species/item ids and immutable object coordinates are adapter
// facts, so generic skill/planner code consumes this table rather than owning
// Red literals itself.
type StaticCaptureSite struct {
	Name        string
	Place       string
	Map         uint8
	X, Y        uint8
	StandX      uint8
	StandY      uint8
	Species     uint8
	Requirement string

	// WakeItem is non-zero when the encounter is initiated by using a key item
	// rather than talking to the overworld object. Red's Route 16 Snorlax is
	// the only current site with that mechanic.
	WakeItem uint8

	// PreferMasterBall is Red capture policy for the uniquely valuable static
	// target. Other sites preserve the Master Ball while ordinary balls remain.
	PreferMasterBall bool
}

const (
	staticMasterBall uint8 = 0x01
	staticUltraBall  uint8 = 0x02
	staticGreatBall  uint8 = 0x03
	staticPokeBall   uint8 = 0x04
	staticPokeFlute  uint8 = 0x49
)

var staticCaptureSites = []StaticCaptureSite{
	{Name: "Route 16 Snorlax", Place: "route 16 snorlax capture", Map: 0x1B, X: 26, Y: 10, StandX: 27, StandY: 10, Species: 0x84, Requirement: "poke_flute", WakeItem: staticPokeFlute},
	{Name: "Articuno", Place: "seafoam articuno", Map: 0xA2, X: 6, Y: 1, StandX: 6, StandY: 2, Species: 0x4A},
	{Name: "Zapdos", Place: "power plant zapdos", Map: 0x53, X: 4, Y: 9, StandX: 4, StandY: 10, Species: 0x4B},
	{Name: "Moltres", Place: "victory road moltres", Map: 0xC2, X: 11, Y: 5, StandX: 11, StandY: 6, Species: 0x49},
	{Name: "Mewtwo", Place: "cerulean cave mewtwo", Map: 0xE3, X: 27, Y: 13, StandX: 27, StandY: 14, Species: 0x83, PreferMasterBall: true},
}

// StaticCaptureSites returns a copy so adapter facts cannot be mutated by a
// planner or skill consumer.
func StaticCaptureSites() []StaticCaptureSite {
	out := make([]StaticCaptureSite, len(staticCaptureSites))
	copy(out, staticCaptureSites)
	return out
}

func StaticCaptureSiteForSpecies(species uint8) (StaticCaptureSite, bool) {
	for _, site := range staticCaptureSites {
		if site.Species == species {
			return site, true
		}
	}
	return StaticCaptureSite{}, false
}

// StaticCaptureBallOrder is Red's deterministic ball preference for a static
// encounter. Raw item ids remain here at the adapter-data boundary.
func StaticCaptureBallOrder(site StaticCaptureSite) []uint8 {
	if site.PreferMasterBall {
		return []uint8{staticMasterBall, staticUltraBall, staticGreatBall, staticPokeBall}
	}
	return []uint8{staticUltraBall, staticGreatBall, staticPokeBall, staticMasterBall}
}

// WildCaptureBallOrder is Red's ball preference for ordinary wild catches:
// strongest first, because a better ball is the largest lever on each throw's
// catch chance. The Master Ball is deliberately absent: it is reserved for
// one-time static encounters, where a miss can consume the only chance.
func WildCaptureBallOrder() []uint8 {
	return []uint8{staticUltraBall, staticGreatBall, staticPokeBall}
}
