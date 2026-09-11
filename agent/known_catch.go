package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const knownCatchLimit = 8

type knownCatchHabitat struct {
	Species SpeciesID
	Place   PlaceID
	Hops    int
}

// appendKnownCatchObjectives turns already-observed wild tables into strategic
// catch transitions. Offer intentionally exposes only immediately local catch
// actions because it has no ROM adapter. The Red-owned offer path does have
// the ROM, so once a map has been visited we can reconstruct the same wild
// table Observation showed on that map and offer a bounded, executable
// "travel there, then hunt" objective from later maps.
//
// This does not reveal unseen habitats: only Knowledge.Visited maps are read.
func appendKnownCatchObjectives(romData []byte, obs Observation, known *Knowledge, out []Objective) []Objective {
	if known == nil || !hasBalls(obs) || obs.PartyCount >= 6 {
		return out
	}

	alreadyOffered := map[SpeciesID]bool{}
	owned := map[SpeciesID]bool{}
	for _, mon := range obs.Party {
		if mon.Species != "" {
			owned[mon.Species] = true
		}
	}
	for _, o := range out {
		if o.Kind == KindCatch && o.Species != "" {
			alreadyOffered[o.Species] = true
		}
	}

	hops := mapHops(known.Adjacency, obs.Map)
	best := map[SpeciesID]knownCatchHabitat{}
	for mapID := range known.Visited {
		if mapID == obs.Map {
			continue // local catches already came from Offer.
		}
		distance, reachable := hops[mapID]
		if len(known.Adjacency) > 0 && !reachable {
			continue
		}
		place, ok := catchPlaceOnMap(mapID)
		if !ok {
			continue
		}
		wild, err := skill.WildGrass(romData, mapID)
		if err != nil || len(wild) == 0 {
			continue
		}
		for _, encounter := range wild {
			sp, ok := SpeciesByName(encounter.Name)
			if !ok || owned[sp] || alreadyOffered[sp] {
				continue
			}
		candidate := knownCatchHabitat{Species: sp, Place: place, Hops: distance}
		prev, exists := best[sp]
		if !exists || candidate.Hops < prev.Hops || (candidate.Hops == prev.Hops && candidate.Place < prev.Place) {
			best[sp] = candidate
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

func catchPlaceOnMap(mapID uint8) (PlaceID, bool) {
	for _, name := range skill.PlaceNames() {
		dest, ok := skill.Place(name)
		if ok && dest.Map == mapID {
			return PlaceID(name), true
		}
	}
	return "", false
}
