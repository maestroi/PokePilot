package agent

import (
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const (
	dexFossilIntent         = "dex-fossil-revival"
	dexFossilObjectiveLimit = 2
)

func appendDexFossilObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	if len(obs.Dex.Targets) == 0 {
		return out
	}
	owned := pokedexOwnedSet(obs)
	already := map[SpeciesID]bool{}
	for _, objective := range out {
		if objective.Kind == KindCatch && objective.Species != "" {
			already[objective.Species] = true
		}
	}

	place := PlaceID(skill.FossilRevivalPlace())
	blocked := dexCatchBlockedPlaces(obs)
	var adjacency map[uint8][]uint8
	var hops map[uint8]int
	if known != nil {
		adjacency = known.Adjacency
		hops = mapHops(adjacency, obs.Map)
	}
	if _, ok := dexCatchPlaceDistance(obs, place, blocked, hops, adjacency); !ok {
		return out
	}

	added := 0
	for _, entry := range obs.Dex.Targets {
		if added >= dexFossilObjectiveLimit || owned[entry.Species] || already[entry.Species] {
			continue
		}
		for _, src := range entry.Sources {
			if src.Kind != AcquireFossil || src.Requirement == "" {
				continue
			}
			item := strings.ReplaceAll(src.Requirement, "_", " ")
			if bagQuantity(obs, item) <= 0 {
				continue
			}
			out = append(out, Objective{
				Kind:    KindCatch,
				Species: entry.Species,
				Place:   place,
				Intent:  dexFossilIntent,
				Flee:    true,
				Note:    "(revive " + item + " at Cinnabar Lab and verify permanent Pokédex ownership)",
			})
			already[entry.Species] = true
			added++
			break
		}
	}
	return out
}
