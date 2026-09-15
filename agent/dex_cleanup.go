package agent

import (
	"fmt"
	"sort"
	"strings"
)

const dexCleanupObjectiveLimit = 8

type dexCleanupCandidate struct {
	objective Objective
	target    SpeciesID
	method    string
	cost      int
}

// prioritizeDexCleanupObjectives turns the already-legal Dex objective set into
// a deterministic post-story cleanup window. It deliberately runs only after
// the durable Hall-of-Fame fact is set: normal story play keeps every existing
// opportunistic objective and its historical ordering.
//
// Each acquisition executor remains responsible for legality. This layer only
// compares alternatives that those executors have already offered (for example
// catching an evolved form directly versus evolving an owned base), keeps the
// cheapest supported route per missing species, and bounds the resulting
// cleanup menu. Enabling/safety objectives such as buying a required stone,
// collecting a rod, healing, or storage recovery are never discarded here.
func prioritizeDexCleanupObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	if !obs.Story.Has(ProgressMainStoryComplete) || len(obs.Dex.Targets) == 0 {
		return out
	}

	targets := make(map[SpeciesID]DexEntry, len(obs.Dex.Targets))
	for _, entry := range obs.Dex.Targets {
		targets[entry.Species] = entry
	}

	passthrough := make([]Objective, 0, len(out))
	best := make(map[SpeciesID]dexCleanupCandidate, len(targets))
	for _, objective := range out {
		target, method, ok := dexCleanupObjectiveTarget(obs, targets, objective)
		if !ok {
			passthrough = append(passthrough, objective)
			continue
		}
		candidate := dexCleanupCandidate{
			objective: objective,
			target:    target,
			method:    method,
			cost:      dexCleanupObjectiveCost(obs, known, objective, method),
		}
		previous, exists := best[target]
		if !exists || betterDexCleanupCandidate(candidate, previous) {
			best[target] = candidate
		}
	}

	candidates := make([]dexCleanupCandidate, 0, len(best))
	for _, candidate := range best {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return betterDexCleanupCandidate(candidates[i], candidates[j])
	})
	if len(candidates) > dexCleanupObjectiveLimit {
		candidates = candidates[:dexCleanupObjectiveLimit]
	}

	for i, candidate := range candidates {
		objective := candidate.objective
		annotation := fmt.Sprintf("[dex cleanup %d: %s via %s]", i+1, strings.ToUpper(string(candidate.target)), candidate.method)
		if objective.Note == "" {
			objective.Note = annotation
		} else {
			objective.Note += " " + annotation
		}
		passthrough = append(passthrough, objective)
	}
	return passthrough
}

func dexCleanupObjectiveTarget(obs Observation, targets map[SpeciesID]DexEntry, objective Objective) (SpeciesID, string, bool) {
	switch objective.Kind {
	case KindCatch:
		if _, ok := targets[objective.Species]; !ok || objective.Species == "" {
			return "", "", false
		}
		return objective.Species, dexCatchMethod(objective.Intent), true
	case KindTrain:
		if objective.Intent != "dex-evolution" || objective.Species == "" {
			return "", "", false
		}
		for _, entry := range targets {
			for _, source := range entry.Sources {
				if source.Kind != AcquireLevelEvo || source.From != objective.Species {
					continue
				}
				if source.Level == 0 || objective.Level >= source.Level {
					return entry.Species, AcquireLevelEvo, true
				}
			}
		}
	case KindUseItem:
		if objective.Intent != "dex-evolution" || objective.Slot < 0 || objective.Slot >= len(obs.Party) {
			return "", "", false
		}
		base := obs.Party[objective.Slot].Species
		for _, entry := range targets {
			for _, source := range entry.Sources {
				if source.Kind == AcquireItemEvo && source.From == base && strings.EqualFold(string(source.Item), string(objective.Item)) {
					return entry.Species, AcquireItemEvo, true
				}
			}
		}
	}
	return "", "", false
}

// dexCleanupObjectiveCost is deliberately coarse and deterministic. Travel is
// the dominant cost. Within one location, deterministic item/level evolutions
// can beat a hunt; a nearby direct encounter can beat many levels of training.
// Finite Moon Stones receive an extra penalty so a repeatable direct catch is
// preferred when it is genuinely cheaper.
func dexCleanupObjectiveCost(obs Observation, known *Knowledge, objective Objective, method string) int {
	travel := dexCleanupTravelCost(obs, known, objective.Place, method)
	switch objective.Kind {
	case KindUseItem:
		resource := 1
		if !purchasableDexEvolutionStone(objective.Item) {
			resource = 6
		}
		return travel*12 + 1 + resource
	case KindTrain:
		delta := 1
		if objective.Slot >= 0 && objective.Slot < len(obs.Party) {
			level := int(obs.Party[objective.Slot].Level)
			if int(objective.Level) > level {
				delta = int(objective.Level) - level
			}
		}
		return travel*12 + 3 + delta
	case KindCatch:
		work := 6
		switch method {
		case AcquireGift:
			work = 1
		case AcquireStatic:
			work = 2
		case AcquireInGameTrade:
			work = 3
		case AcquireFossil:
			work = 4
		case "game_corner_prize":
			work = 5
		case AcquireWildGrass:
			work = 6
		case AcquireFishing, AcquireWildWater:
			work = 7
		case "safari":
			work = 9
		}
		return travel*12 + work
	default:
		return travel*12 + 100
	}
}

func dexCleanupTravelCost(obs Observation, known *Knowledge, place PlaceID, method string) int {
	if place == "" || place == obs.Location {
		return 0
	}
	// Safari objectives name the habitat, but execution owns the paid gate and
	// internal session routing. Compare travel to Fuchsia, the same access point
	// used when the objective is offered.
	if method == "safari" {
		place = dexSafariAccessPlace
	}
	blocked := dexCatchBlockedPlaces(obs)
	hops := map[uint8]int{}
	var adjacency map[uint8][]uint8
	if known != nil {
		adjacency = known.nativeAdjacency()
		hops = mapHops(adjacency, obs.Map)
	}
	if distance, ok := dexCatchPlaceDistance(obs, place, blocked, hops, adjacency); ok {
		return distance
	}
	// The provider has already proved execution legal; missing graph evidence in
	// a synthetic/legacy observation should rank the route last, not delete it.
	return 20
}

func betterDexCleanupCandidate(candidate, previous dexCleanupCandidate) bool {
	if candidate.cost != previous.cost {
		return candidate.cost < previous.cost
	}
	if candidate.target != previous.target {
		return candidate.target < previous.target
	}
	if candidate.method != previous.method {
		return candidate.method < previous.method
	}
	return candidate.objective.String() < previous.objective.String()
}
