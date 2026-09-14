package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const (
	dexStaticIntent = "dex-static"
	dexStaticLimit  = 3
)

func hasStaticBalls(obs Observation) bool {
	for _, item := range obs.Bag {
		switch strings.ToLower(strings.TrimSpace(item.Name)) {
		case "master ball", "ultra ball", "great ball", "pokeball", "poke ball":
			if item.Quantity > 0 {
				return true
			}
		}
	}
	return false
}

func staticSpeciesID(raw uint8) (SpeciesID, bool) {
	switch raw {
	case 0x84:
		return "snorlax", true
	case 0x4A:
		return "articuno", true
	case 0x4B:
		return "zapdos", true
	case 0x49:
		return "moltres", true
	case 0x83:
		return "mewtwo", true
	default:
		return "", false
	}
}

func staticObjectiveQuarantined(known *Knowledge, objective Objective) bool {
	if known == nil {
		return false
	}
	failure, ok := known.Failures[objective.String()]
	if !ok {
		return false
	}
	last := strings.ToLower(failure.Last)
	return strings.Contains(last, "static capture attempts exhausted") || strings.Contains(last, "one-time static source unavailable")
}

func appendDexStaticObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	if len(obs.Dex.Targets) == 0 || !hasStaticBalls(obs) {
		return out
	}
	owned := pokedexOwnedSet(obs)
	already := map[SpeciesID]bool{}
	for _, objective := range out {
		if objective.Kind == KindCatch && objective.Species != "" {
			already[objective.Species] = true
		}
	}

	sites := map[SpeciesID]skill.StaticCaptureSite{}
	for _, site := range skill.StaticCaptureSites() {
		id, ok := staticSpeciesID(site.Species)
		if ok {
			sites[id] = site
		}
	}
	blocked := dexCatchBlockedPlaces(obs)
	var adjacency map[uint8][]uint8
	var hops map[uint8]int
	if known != nil {
		adjacency = known.Adjacency
		hops = mapHops(adjacency, obs.Map)
	}

	added := 0
	for _, entry := range obs.Dex.Targets {
		if added >= dexStaticLimit || owned[entry.Species] || already[entry.Species] {
			continue
		}
		site, ok := sites[entry.Species]
		if !ok {
			continue
		}
		isStatic := false
		for _, source := range entry.Sources {
			if source.Kind == AcquireStatic {
				isStatic = true
				break
			}
		}
		if !isStatic {
			continue
		}
		if site.Requirement == "poke_flute" && bagQuantity(obs, "poke flute") <= 0 {
			continue
		}
		place := PlaceID(site.Place)
		if _, ok := dexCatchPlaceDistance(obs, place, blocked, hops, adjacency); !ok {
			continue
		}
		objective := Objective{
			Kind:    KindCatch,
			Species: entry.Species,
			Place:   place,
			Intent:  dexStaticIntent,
			Flee:    true,
			Note:    fmt.Sprintf("(one-time static %s; checkpoint + bounded RNG-phase retries; rollback on every failed capture)", site.Name),
		}
		if staticObjectiveQuarantined(known, objective) {
			continue
		}
		out = append(out, objective)
		already[entry.Species] = true
		added++
	}
	return out
}
