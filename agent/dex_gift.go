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
// planner from inventing execution for Lapras/Dojo/Porygon merely because the
// catalog knows those species are locally obtainable.
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
	for _, entry := range obs.Dex.Targets {
		if added >= dexGiftObjectiveLimit || owned[entry.Species] || already[entry.Species] {
			continue
		}
		for _, src := range entry.Sources {
			if !dexGiftSourceExecutable(entry.Species, src) {
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
			already[entry.Species] = true
			added++
			break
		}
	}
	return out
}

func dexGiftSourceExecutable(species SpeciesID, src DexSource) bool {
	// Eevee is the first scripted gift with a dedicated execution contract.
	// Add later gifts here only together with their concrete skill path.
	return species == "eevee" && src.Kind == AcquireGift && src.Place == "celadon mansion eevee"
}
