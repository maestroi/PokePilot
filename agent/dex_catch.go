package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const dexCatchLimit = 8

type dexCatchHabitat struct {
	Species SpeciesID
	Place   PlaceID
	Hops    int
}

// appendDexCatchObjectives turns catalog grass sources into bounded
// travel-then-hunt objectives. Only wild_grass with no extra requirement is
// executable today (Catch hunts the current map's grass). Water, rods,
// Safari, statics, gifts, and trades stay catalog facts until those skills
// exist. Story gates still apply: a source behind a blocked transition is
// not offered.
func appendDexCatchObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	if !hasBalls(obs) || len(obs.Dex.Targets) == 0 {
		return out
	}

	alreadyOffered := map[SpeciesID]bool{}
	owned := pokedexOwnedSet(obs)
	for _, o := range out {
		if o.Kind == KindCatch && o.Species != "" {
			alreadyOffered[o.Species] = true
		}
	}

	blocked := dexCatchBlockedPlaces(obs)
	hops := map[uint8]int{}
	var adjacency map[uint8][]uint8
	if known != nil {
		adjacency = known.Adjacency
		hops = mapHops(adjacency, obs.Map)
	}

	best := map[SpeciesID]dexCatchHabitat{}
	for _, entry := range obs.Dex.Targets {
		if owned[entry.Species] || alreadyOffered[entry.Species] {
			continue
		}
		for _, src := range entry.Sources {
			place, distance, ok := dexCatchGrassSource(obs, src, blocked, hops, adjacency)
			if !ok {
				continue
			}
			candidate := dexCatchHabitat{Species: entry.Species, Place: place, Hops: distance}
			prev, exists := best[entry.Species]
			if !exists || candidate.Hops < prev.Hops || (candidate.Hops == prev.Hops && candidate.Place < prev.Place) {
				best[entry.Species] = candidate
			}
		}
	}

	candidates := make([]dexCatchHabitat, 0, len(best))
	for _, candidate := range best {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Hops != candidates[j].Hops {
			return candidates[i].Hops < candidates[j].Hops
		}
		if candidates[i].Place != candidates[j].Place {
			return candidates[i].Place < candidates[j].Place
		}
		return candidates[i].Species < candidates[j].Species
	})
	if len(candidates) > dexCatchLimit {
		candidates = candidates[:dexCatchLimit]
	}

	for _, candidate := range candidates {
		out = append(out, Objective{
			Kind:    KindCatch,
			Species: candidate.Species,
			Place:   candidate.Place,
			Flee:    true,
			Note: fmt.Sprintf("(dex target: %s; travel included)",
				strings.ToUpper(string(candidate.Place))),
		})
	}
	return out
}

func dexCatchGrassSource(obs Observation, src DexSource, blocked map[PlaceID]bool, hops map[uint8]int, adjacency map[uint8][]uint8) (PlaceID, int, bool) {
	if src.Kind != AcquireWildGrass || src.Requirement != "" || src.Place == "" {
		return "", 0, false
	}
	if blocked[src.Place] {
		return "", 0, false
	}
	dest, ok := skill.Place(string(src.Place))
	if !ok {
		return "", 0, false
	}
	if journeyProgressionBlocked(obs, dest.Map) || placeProgressionBlocked(obs, string(src.Place)) {
		return "", 0, false
	}
	distance, reachable := hops[dest.Map]
	if dest.Map == obs.Map {
		distance, reachable = 0, true
	}
	if len(adjacency) > 0 && !reachable {
		return "", 0, false
	}
	return src.Place, distance, true
}

func dexCatchBlockedPlaces(obs Observation) map[PlaceID]bool {
	out := map[PlaceID]bool{}
	for _, name := range obs.Unroutable {
		out[PlaceID(name)] = true
	}
	for _, blockage := range obs.RouteBlockages {
		if blockage.Destination != "" {
			out[blockage.Destination] = true
		}
	}
	return out
}
