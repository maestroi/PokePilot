package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
)

const knownCatchLimit = 8

type knownCatchHabitat struct {
	Species SpeciesID
	Place   PlaceID
	Hops    int
}

type wildGrassLookup func([]byte, uint8) ([]skill.WildSpecies, error)

// appendKnownCatchObjectives turns already-observed wild tables into strategic
// catch transitions. Offer intentionally exposes only immediately local catch
// actions because it has no ROM adapter. The Red-owned offer path does have
// the ROM, so once a map has been visited we can reconstruct the same wild
// table Observation showed on that map and offer a bounded, executable
// "travel there, then hunt" objective from later maps.
//
// This does not reveal unseen habitats: only Knowledge.Visited locations are
// projected back through Red's adapter before ROM lookup.
func appendKnownCatchObjectives(m *emu.Emu, romData []byte, obs Observation, known *Knowledge, out []Objective) []Objective {
	return appendKnownCatchObjectivesWithWild(m, romData, obs, known, out, skill.WildGrass)
}

func appendKnownCatchObjectivesWithWild(m *emu.Emu, romData []byte, obs Observation, known *Knowledge, out []Objective, wildFor wildGrassLookup) []Objective {
	if known == nil || wildFor == nil || !hasBalls(obs) || obs.PartyCount >= 6 {
		return out
	}

	// GoTo enforces capability gates (Snorlax, Cut, Surf, badges, ...) that
	// the plain map-adjacency graph below knows nothing about; without this,
	// a route only reachable once a story gate clears looks identical to one
	// reachable right now, and the offered objective fails at travel time
	// every time it is picked. reachableMaps is nil (not filtered) when no
	// live state is available, e.g. in unit tests.
	reachableMaps, err := skill.ReachableMaps(m, romData)
	if err != nil {
		reachableMaps = nil
	}

	alreadyOffered := map[SpeciesID]bool{}
	owned := pokedexOwnedSet(obs)
	for _, o := range out {
		if o.Kind == KindCatch && o.Species != "" {
			alreadyOffered[o.Species] = true
		}
	}

	adjacency := known.nativeAdjacency()
	hops := mapHops(adjacency, obs.Map)
	best := map[SpeciesID]knownCatchHabitat{}
	seenNative := map[uint8]bool{}
	for location := range known.Visited {
		mapID, ok := redNativeMapForLocation(obs.GameID, known, location)
		if !ok || seenNative[mapID] || mapID == obs.Map {
			continue // local catches already came from Offer.
		}
		seenNative[mapID] = true
		distance, reachable := hops[mapID]
		if len(adjacency) > 0 && !reachable {
			continue
		}
		if reachableMaps != nil && !reachableMaps[mapID] {
			continue
		}
		place, ok := catchPlaceOnMap(mapID)
		if !ok {
			continue
		}
		wild, err := wildFor(romData, mapID)
		if err != nil || len(wild) == 0 {
			continue
		}
		for _, encounter := range wild {
			name, ok := SpeciesName(encounter.ID)
			if !ok {
				continue
			}
			sp, ok := SpeciesByName(name)
			if !ok || owned[sp] || alreadyOffered[sp] {
				continue
			}
			candidate := knownCatchHabitat{Species: sp, Place: place, Hops: distance}
			prev, exists := best[sp]
			if !exists || candidate.Hops < prev.Hops || (candidate.Hops == prev.Hops && candidate.Place < prev.Place) {
				best[sp] = candidate
			}
		}
	}

	candidates := make([]knownCatchHabitat, 0, len(best))
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
	if len(candidates) > knownCatchLimit {
		candidates = candidates[:knownCatchLimit]
	}

	for _, candidate := range candidates {
		out = append(out, Objective{
			Kind:    KindCatch,
			Species: candidate.Species,
			Place:   candidate.Place,
			Flee:    true,
			Note: fmt.Sprintf("(known habitat: %s; travel included)",
				strings.ToUpper(string(candidate.Place))),
		})
	}
	return out
}

func redNativeMapForLocation(gameID game.GameID, known *Knowledge, location LocationID) (uint8, bool) {
	if known != nil {
		for native, semantic := range known.nativeLocations {
			if semantic == location {
				return native, true
			}
		}
	}
	for i := 0; i <= 0xff; i++ {
		id := uint8(i)
		if redLocationID(gameID, id) == location {
			return id, true
		}
	}
	return 0, false
}

func catchPlaceOnMap(mapID uint8) (PlaceID, bool) {
	for _, name := range skill.PlaceNames() {
		dest, ok := skill.Place(name)
		if !ok || dest.Map != mapID {
			continue
		}
		// Catch travel must target habitat semantics, never an unrelated exact
		// service/script anchor that merely happens to share the map.
		if dest.Kind == skill.DestinationMap || dest.Kind == skill.DestinationArea {
			return PlaceID(name), true
		}
	}
	return "", false
}
