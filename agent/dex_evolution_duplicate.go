package agent

import (
	"fmt"
	"sort"
	"strings"
)

const dexDuplicateEvolutionBaseLimit = 3

// appendDexDuplicateEvolutionBaseObjectives prepares a second individual for a
// branched evolution family after the base species has already been owned but
// is no longer in the party. Only repeatable acquisition sources are eligible:
// grass, fishing, Surf water, or Safari. One-time gifts/statics are never
// fabricated as repeatable just to satisfy a branch.
func appendDexDuplicateEvolutionBaseObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	if len(obs.Dex.Targets) == 0 {
		return out
	}

	owned := pokedexOwnedSet(obs)
	branches := dexEvolutionBranchCounts(obs.Dex)
	entries := dexCatalogEntriesBySpecies(obs.Dex)
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

	best := map[SpeciesID]dexCatchHabitat{}
	for _, target := range obs.Dex.Targets {
		for _, evo := range target.Sources {
			if evo.From == "" || (evo.Kind != AcquireLevelEvo && evo.Kind != AcquireItemEvo) {
				continue
			}
			base := evo.From
			if branches[base] < 2 || !owned[base] || already[base] {
				continue
			}
			if _, _, inParty := partySpeciesSlot(obs.Party, base); inParty {
				continue
			}
			entry, ok := entries[base]
			if !ok {
				continue
			}
			for _, src := range entry.Sources {
				candidate, ok := dexCatchSource(obs, base, src, blocked, hops, adjacency)
				if !ok {
					continue
				}
				previous, exists := best[base]
				if !exists || betterDexCatchHabitat(candidate, previous) {
					best[base] = candidate
				}
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
	if len(candidates) > dexDuplicateEvolutionBaseLimit {
		candidates = candidates[:dexDuplicateEvolutionBaseLimit]
	}

	for _, candidate := range candidates {
		note := fmt.Sprintf("(dex duplicate base: catch another %s at %s for a remaining evolution branch)",
			strings.ToUpper(string(candidate.Species)), strings.ToUpper(string(candidate.Place)))
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

func dexEvolutionBranchCounts(cat DexCatalog) map[SpeciesID]int {
	targets := map[SpeciesID]map[SpeciesID]bool{}
	visit := func(entries []DexEntry) {
		for _, entry := range entries {
			for _, src := range entry.Sources {
				if src.From == "" || (src.Kind != AcquireLevelEvo && src.Kind != AcquireItemEvo) {
					continue
				}
				if targets[src.From] == nil {
					targets[src.From] = map[SpeciesID]bool{}
				}
				targets[src.From][entry.Species] = true
			}
		}
	}
	visit(cat.Owned)
	visit(cat.Targets)
	visit(cat.Unavailable)
	out := map[SpeciesID]int{}
	for base, species := range targets {
		out[base] = len(species)
	}
	return out
}

func dexCatalogEntriesBySpecies(cat DexCatalog) map[SpeciesID]DexEntry {
	out := map[SpeciesID]DexEntry{}
	for _, entries := range [][]DexEntry{cat.Owned, cat.Targets, cat.Unavailable} {
		for _, entry := range entries {
			if _, exists := out[entry.Species]; !exists {
				out[entry.Species] = entry
			}
		}
	}
	return out
}
