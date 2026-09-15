package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const (
	dexCatchLimit         = 8
	dexFishingIntent      = "dex-fishing"
	dexWaterIntent        = "dex-water"
	dexSafariIntent       = "dex-safari"
	dexGlobalFishingPlace = PlaceID("vermilion city")
	dexSafariAccessPlace  = PlaceID("fuchsia city")
	dexSafariFee          = 500
)

type dexCatchHabitat struct {
	Species SpeciesID
	Place   PlaceID
	Hops    int
	Intent  string
	Rod     ItemID
}

// appendDexCatchObjectives turns executable catalog sources into bounded
// travel-then-acquire objectives. Grass hunts use Catch directly; fishing,
// Surf water and Safari grass use deterministic Red-owned acquisition skills.
// Statics, gifts and trades remain catalog facts until their execution skills
// exist. Story gates still apply: a source behind a blocked transition is not
// offered.
func appendDexCatchObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	if len(obs.Dex.Targets) == 0 {
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
			candidate, ok := dexCatchSource(obs, entry.Species, src, blocked, hops, adjacency)
			if !ok {
				continue
			}
			prev, exists := best[entry.Species]
			if !exists || betterDexCatchHabitat(candidate, prev) {
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
		if dexCatchMethodRank(candidates[i]) != dexCatchMethodRank(candidates[j]) {
			return dexCatchMethodRank(candidates[i]) < dexCatchMethodRank(candidates[j])
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
		note := fmt.Sprintf("(dex target: %s; travel included)", strings.ToUpper(string(candidate.Place)))
		switch candidate.Intent {
		case dexFishingIntent:
			note = fmt.Sprintf("(dex fishing: %s at %s; travel included)",
				strings.ToUpper(string(candidate.Rod)), strings.ToUpper(string(candidate.Place)))
		case dexWaterIntent:
			note = fmt.Sprintf("(dex Surf hunt at %s; travel and verified Surf entry included)",
				strings.ToUpper(string(candidate.Place)))
		case dexSafariIntent:
			note = fmt.Sprintf("(dex Safari hunt at %s; paid session, Safari Balls and exit/re-entry are owned by execution)",
				strings.ToUpper(string(candidate.Place)))
		}
		out = append(out, Objective{
			Kind:    KindCatch,
			Species: candidate.Species,
			Place:   candidate.Place,
			Item:    candidate.Rod,
			Intent:  candidate.Intent,
			Flee:    true,
			Note:    note,
		})
	}
	return out
}

func dexCatchSource(obs Observation, species SpeciesID, src DexSource, blocked map[PlaceID]bool, hops map[uint8]int, adjacency map[uint8][]uint8) (dexCatchHabitat, bool) {
	if place, distance, ok := dexCatchGrassSource(obs, src, blocked, hops, adjacency); ok {
		return dexCatchHabitat{Species: species, Place: place, Hops: distance}, true
	}
	place, distance, rod, ok := dexCatchFishingSource(obs, src, blocked, hops, adjacency)
	if ok {
		return dexCatchHabitat{Species: species, Place: place, Hops: distance, Intent: dexFishingIntent, Rod: rod}, true
	}
	place, distance, ok = dexCatchWaterSource(obs, src, blocked, hops, adjacency)
	if ok {
		return dexCatchHabitat{Species: species, Place: place, Hops: distance, Intent: dexWaterIntent}, true
	}
	place, distance, ok = dexCatchSafariSource(obs, src, blocked, hops, adjacency)
	if ok {
		return dexCatchHabitat{Species: species, Place: place, Hops: distance, Intent: dexSafariIntent}, true
	}
	return dexCatchHabitat{}, false
}

func betterDexCatchHabitat(candidate, previous dexCatchHabitat) bool {
	if candidate.Hops != previous.Hops {
		return candidate.Hops < previous.Hops
	}
	if dexCatchMethodRank(candidate) != dexCatchMethodRank(previous) {
		return dexCatchMethodRank(candidate) < dexCatchMethodRank(previous)
	}
	if candidate.Place != previous.Place {
		return candidate.Place < previous.Place
	}
	return candidate.Rod < previous.Rod
}

func dexCatchMethodRank(candidate dexCatchHabitat) int {
	switch candidate.Intent {
	case dexFishingIntent:
		return 1
	case dexWaterIntent:
		return 2
	case dexSafariIntent:
		return 3
	default:
		return 0
	}
}

func dexCatchGrassSource(obs Observation, src DexSource, blocked map[PlaceID]bool, hops map[uint8]int, adjacency map[uint8][]uint8) (PlaceID, int, bool) {
	if !hasBalls(obs) || src.Kind != AcquireWildGrass || src.Requirement != "" || src.Place == "" {
		return "", 0, false
	}
	return dexCatchPlace(obs, src.Place, blocked, hops, adjacency)
}

func dexCatchFishingSource(obs Observation, src DexSource, blocked map[PlaceID]bool, hops map[uint8]int, adjacency map[uint8][]uint8) (PlaceID, int, ItemID, bool) {
	if !hasBalls(obs) || src.Kind != AcquireFishing {
		return "", 0, "", false
	}
	rod, ok := dexFishingRod(src.Requirement)
	if !ok || bagItemQuantity(obs.Bag, rod) <= 0 {
		return "", 0, "", false
	}
	place := src.Place
	if place == "" {
		// Old and Good Rod encounter tables are global in Red. Pick one stable
		// shoreline-capable cleanup map rather than pretending the source is
		// mapless. Owning either rod implies Vermilion is already accessible.
		place = dexGlobalFishingPlace
	}
	distance, ok := dexCatchPlaceDistance(obs, place, blocked, hops, adjacency)
	if !ok {
		return "", 0, "", false
	}
	return place, distance, rod, true
}

func dexCatchWaterSource(obs Observation, src DexSource, blocked map[PlaceID]bool, hops map[uint8]int, adjacency map[uint8][]uint8) (PlaceID, int, bool) {
	if !hasBalls(obs) || src.Kind != AcquireWildWater || src.Requirement != "surf" || src.Place == "" || !dexSurfAvailable(obs) {
		return "", 0, false
	}
	return dexCatchPlace(obs, src.Place, blocked, hops, adjacency)
}

func dexCatchSafariSource(obs Observation, src DexSource, blocked map[PlaceID]bool, hops map[uint8]int, adjacency map[uint8][]uint8) (PlaceID, int, bool) {
	if src.Kind != AcquireWildGrass || src.Place == "" || strings.TrimSpace(src.Requirement) != "safari_zone" {
		return "", 0, false
	}
	mapID, ok := dexPlaceMapID(src.Place)
	if !ok || !safariRequirement(mapID) {
		return "", 0, false
	}
	inside := safariRequirement(obs.Map)
	if !inside && obs.Money < dexSafariFee {
		return "", 0, false
	}
	if inside {
		return src.Place, 0, true
	}
	// Safari maps are intentionally not ordinary Place destinations because
	// a paid finite session must own their routing. Use Fuchsia City as the
	// planner-facing access cost; SafariCatch owns the gate and habitat travel.
	distance, ok := dexCatchPlaceDistance(obs, dexSafariAccessPlace, blocked, hops, adjacency)
	if !ok {
		return "", 0, false
	}
	return src.Place, distance, true
}

func dexPlaceMapID(place PlaceID) (uint8, bool) {
	for i := 0; i <= 0xff; i++ {
		id := uint8(i)
		if mapPlace(id) == place {
			return id, true
		}
	}
	return 0, false
}

func dexSurfAvailable(obs Observation) bool {
	for _, capability := range obs.FieldCapabilities {
		if strings.EqualFold(string(capability.Name), "surf") {
			return capability.Usable || capability.Preparable
		}
	}
	return false
}

func dexFishingRod(requirement string) (ItemID, bool) {
	switch strings.ToLower(strings.TrimSpace(requirement)) {
	case "old_rod":
		return ItemID("old rod"), true
	case "good_rod":
		return ItemID("good rod"), true
	case "super_rod":
		return ItemID("super rod"), true
	default:
		// Combined requirements such as "super_rod,safari_zone" are not
		// executable by ordinary fishing; Safari owns a different battle mode.
		return "", false
	}
}

func dexCatchPlace(obs Observation, place PlaceID, blocked map[PlaceID]bool, hops map[uint8]int, adjacency map[uint8][]uint8) (PlaceID, int, bool) {
	distance, ok := dexCatchPlaceDistance(obs, place, blocked, hops, adjacency)
	return place, distance, ok
}

func dexCatchPlaceDistance(obs Observation, place PlaceID, blocked map[PlaceID]bool, hops map[uint8]int, adjacency map[uint8][]uint8) (int, bool) {
	if place == "" || blocked[place] {
		return 0, false
	}
	dest, ok := skill.Place(string(place))
	if !ok {
		return 0, false
	}
	// If Red is already on this map, no progression transition is being
	// attempted. Current-map collection remains executable unless the adapter
	// explicitly blocks this semantic waypoint (for example Route 12 Snorlax).
	if dest.Map == obs.Map {
		return 0, true
	}
	distance, reachable := hops[dest.Map]
	if len(adjacency) > 0 && !reachable {
		return 0, false
	}
	return distance, true
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
	// Dex acquisition is Red-owned today, so consume the same adapter route
	// requirements as the generic offer pipeline even when this helper is
	// exercised directly in a unit test without OfferWithProgressionEvidence.
	adapter := &redObjectiveAdapter{}
	for _, blockage := range adapter.RouteRequirements(obs) {
		if blockage.Destination != "" {
			out[blockage.Destination] = true
		}
	}
	return out
}
