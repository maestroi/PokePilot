package agent

import (
	"fmt"
	"strings"

	reddata "github.com/maestroi/pokepilot/red/data"
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

// staticRouteCapabilityMissing reports that the last journey to this static
// site was blocked on field capabilities that are still neither usable nor
// repairable. Run-level quarantine reopens a compound catch on movement, so
// without this the planner re-picked Surf-locked Zapdos/Moltres/Mewtwo 700+
// times, walking halfway across Kanto each time (run-d6dokr184ky81).
func staticRouteCapabilityMissing(obs Observation, last string) bool {
	_, rest, ok := strings.Cut(last, "missing capabilities [")
	if !ok {
		return false
	}
	list, _, _ := strings.Cut(rest, "]")
	missing := false
	for _, raw := range strings.Fields(list) {
		link, ok := redRoutePrerequisiteLink(CapabilityID(raw))
		if !ok || link.FieldCapability == "" {
			return false // not a field move we can observe; keep offering
		}
		if fieldCapabilityUsable(obs, link.FieldCapability) || fieldCapabilityRepairReady(obs, link.FieldCapability) {
			return false
		}
		missing = true
	}
	return missing
}

func staticObjectiveQuarantined(known *Knowledge, objective Objective, obs Observation) bool {
	if known == nil {
		return false
	}
	failure, ok := known.Failures[objectiveStorageKey(objective)]
	if !ok {
		// One-release compatibility for v4 checkpoint memory and tests that
		// still seed the old presentation-keyed map directly.
		failure, ok = known.Failures[objective.String()]
	}
	if !ok {
		return false
	}
	last := strings.ToLower(failure.Last)
	return strings.Contains(last, "static capture attempts exhausted") || strings.Contains(last, "one-time static source unavailable") ||
		staticRouteCapabilityMissing(obs, failure.Last)
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

	sites := map[SpeciesID]reddata.StaticCaptureSite{}
	for _, site := range reddata.StaticCaptureSites() {
		id, ok := reddata.Species(site.Species)
		if ok {
			sites[id] = site
		}
	}
	blocked := dexCatchBlockedPlaces(obs)
	var adjacency map[uint16][]uint16
	var hops map[uint16]int
	if known != nil {
		adjacency = known.nativeAdjacency()
		hops = mapHops(adjacency, uint16(obs.Map))
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
		if staticObjectiveQuarantined(known, objective, obs) {
			continue
		}
		out = append(out, objective)
		already[entry.Species] = true
		added++
	}
	return out
}
