package agent

import (
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gen1Route2Map uint8 = 0x0d
	gen1Route3Map uint8 = 0x0e
)

// These two transactions are the first Kanto campaign steps whose mechanics
// are identical across Red, Blue and Yellow once each profile has bound the
// canonical Gen-I memory/ROM view. Keep one executor instead of cloning the
// Parcel/Pokedex and Brock controllers per cartridge.
func gen1AcquirePokedex(m *emu.Emu, romData []byte, policy skill.MovePolicy) error {
	return skill.OaksParcel(m, romData, policy)
}

func gen1DefeatBrock(m *emu.Emu, romData []byte, policy skill.MovePolicy) error {
	return skill.BoulderProgression(m, romData, policy)
}

func gen1EarlyProgressionExecutor(id ProgressID) (redProgressionExecutor, bool) {
	switch id {
	case gen1.ProgressPokedexAcquired:
		return gen1AcquirePokedex, true
	case gen1.ProgressBoulderBadge:
		return gen1DefeatBrock, true
	default:
		return nil, false
	}
}

func gen1ObservationHasBadge(obs Observation, want string) bool {
	for _, badge := range obs.Badges {
		if strings.EqualFold(badge, want) {
			return true
		}
	}
	return false
}

// gen1EarlyProgressionObjectives is the shared Kanto handoff after a
// cartridge-owned opening. The caller decides what proves that its opening is
// complete; after that, the portable facts are identical.
func gen1EarlyProgressionObjectives(obs Observation, openingComplete bool) []Objective {
	if !openingComplete {
		return nil
	}
	if !obs.Story.Has(gen1.ProgressPokedexAcquired) {
		return []Objective{{
			Kind:     KindProgress,
			Progress: gen1.ProgressPokedexAcquired,
			Note:     "(deliver Oak's parcel and acquire the Pokedex)",
		}}
	}
	if !obs.Story.Has(gen1.ProgressBoulderBadge) && !gen1ObservationHasBadge(obs, "Boulder") {
		return []Objective{{
			Kind:     KindProgress,
			Progress: gen1.ProgressBoulderBadge,
			Note:     "(travel through Viridian Forest to Pewter, challenge Brock, and positively verify the Boulder Badge before Route 3)",
		}}
	}
	return nil
}

// gen1EarlyRouteRequirements owns the two shared Kanto world gates. A game may
// add stricter cartridge-specific requirements (Yellow does for its mandatory
// Viridian catch tutorial) without duplicating these common gates.
func gen1EarlyRouteRequirements(obs Observation, namespace string) []RouteBlockage {
	if namespace == "" {
		namespace = "gen1"
	}
	out := make([]RouteBlockage, 0, 2)
	blockMap := func(mapID uint8, transition string, prerequisite RoutePrerequisiteLink) {
		if mapID == obs.Map {
			return
		}
		for _, name := range skill.PlaceNames() {
			destination, ok := skill.Place(name)
			if !ok || destination.Map != mapID {
				continue
			}
			out = append(out, redStoryRouteBlockage(PlaceID(name), transition, prerequisite))
		}
	}

	if !obs.Story.Has(gen1.ProgressBoulderBadge) && !gen1ObservationHasBadge(obs, "Boulder") {
		blockMap(gen1Route3Map, namespace+":story:route3_boulder", RoutePrerequisiteLink{
			Capability: "can_leave_pewter_east",
			Badge:      "Boulder",
			Progress:   gen1.ProgressBoulderBadge,
		})
	}
	if !obs.Story.Has(gen1.ProgressPokedexAcquired) {
		blockMap(gen1Route2Map, namespace+":story:route2_pokedex", RoutePrerequisiteLink{
			Progress: gen1.ProgressPokedexAcquired,
		})
	}
	return dedupeRouteBlockages(out)
}
