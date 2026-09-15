package agent

import (
	"fmt"
	"strings"
)

const (
	dexGiftIntent         = "dex-gift"
	dexGiftObjectiveLimit = 4
)

// appendDexGiftObjectives exposes only scripted gifts with a deterministic
// executor. Scripted sources may be present in the catalog before their menu
// mechanics are implemented; keeping this allow-list explicit prevents the
// planner from inventing execution for Porygon merely because the catalog
// knows it is locally obtainable. Mutually exclusive gift groups expose only
// one deterministic objective per observation so a multi-step plan cannot
// queue both branches of a one-time choice.
func appendDexGiftObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	if len(obs.Dex.Targets) == 0 {
		return out
	}
	owned := pokedexOwnedSet(obs)
	already := map[SpeciesID]bool{}
	for _, o := range out {
		if o.Kind == KindCatch && o.Species != "" {
			already[o.Species] = true
		}
	}

	blocked := dexCatchBlockedPlaces(obs)
	hops := map[uint8]int{}
	var adjacency map[uint8][]uint8
	if known != nil {
		adjacency = known.Adjacency
		hops = mapHops(adjacency, obs.Map)
	}

	added := 0
	exclusiveOffered := map[string]bool{}
	for _, entry := range obs.Dex.Targets {
		if added >= dexGiftObjectiveLimit || owned[entry.Species] || already[entry.Species] {
			continue
		}
		for _, src := range entry.Sources {
			if src.ExclusiveGroup != "" && exclusiveOffered[src.ExclusiveGroup] {
				continue
			}
			if !dexGiftSourceExecutable(obs, entry.Species, src) {
				continue
			}
			if _, ok := dexCatchPlaceDistance(obs, src.Place, blocked, hops, adjacency); !ok {
				continue
			}
			out = append(out, Objective{
				Kind:    KindCatch,
				Species: entry.Species,
				Place:   src.Place,
				Intent:  dexGiftIntent,
				Flee:    true,
				Note: fmt.Sprintf("(dex gift: receive %s at %s; collection storage and nickname handling are verified)",
					strings.ToUpper(string(entry.Species)), strings.ToUpper(string(src.Place))),
			})
			if src.ExclusiveGroup != "" {
				exclusiveOffered[src.ExclusiveGroup] = true
			}
			already[entry.Species] = true
			added++
			break
		}
	}
	return out
}

func dexGiftSourceExecutable(obs Observation, species SpeciesID, src DexSource) bool {
	if src.Kind != AcquireGift {
		return false
	}
	owned := pokedexOwnedSet(obs)
	switch species {
	case "eevee":
		return src.Place == "celadon mansion eevee" && src.Requirement == ""
	case "lapras":
		// Card Key is acquired inside Silph only after Saffron access is open,
		// so owning it is a durable prerequisite for the dedicated rival-room
		// route and stronger evidence than a coarse map reachability guess.
		return src.Place == "silph co lapras" && src.Requirement == "card_key" && bagItemQuantity(obs.Bag, ItemID("card key")) > 0
	case "hitmonlee":
		// The Dojo is inside Saffron. Map-hop reachability alone can include
		// Saffron's interior maps before the guards have accepted a drink, so
		// do not offer the one-time prize until the semantic gate is open.
		return src.Place == "fighting dojo hitmonlee" &&
			src.Requirement == dexRequirementSaffronGateOpen &&
			src.ExclusiveGroup == "fighting_dojo" &&
			obs.Story.Has(ProgressSaffronGateOpen) &&
			!owned["hitmonchan"]
	case "hitmonchan":
		return src.Place == "fighting dojo hitmonchan" &&
			src.Requirement == dexRequirementSaffronGateOpen &&
			src.ExclusiveGroup == "fighting_dojo" &&
			obs.Story.Has(ProgressSaffronGateOpen) &&
			!owned["hitmonlee"]
	default:
		return false
	}
}
