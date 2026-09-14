package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/skill"
)

const dexGameCornerIntent = "dex-game-corner-porygon"

func appendDexGameCornerObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	if len(obs.Dex.Targets) == 0 || pokedexOwnedSet(obs)["porygon"] {
		return out
	}
	for _, o := range out {
		if o.Kind == KindCatch && o.Species == "porygon" {
			return out
		}
	}

	var source *DexSource
	for _, entry := range obs.Dex.Targets {
		if entry.Species != "porygon" {
			continue
		}
		for i := range entry.Sources {
			src := &entry.Sources[i]
			if src.Kind == AcquireGift && src.Requirement == "game_corner" {
				source = src
				break
			}
		}
	}
	if source == nil {
		return out
	}

	// This is deliberately conservative because Observation does not expose the
	// Coin Case counter. Y200000 is the exact worst case from zero coins (200
	// purchases at Y1000); an executor with existing coins may spend less. Never
	// offer an objective that is guaranteed to fail and then be retried forever.
	if obs.Money < skill.MaxPorygonCoinBudget() {
		return out
	}

	place := PlaceID(skill.PorygonPrizePlace())
	blocked := dexCatchBlockedPlaces(obs)
	var adjacency map[uint8][]uint8
	var hops map[uint8]int
	if known != nil {
		adjacency = known.Adjacency
		hops = mapHops(adjacency, obs.Map)
	}
	if _, ok := dexCatchPlaceDistance(obs, place, blocked, hops, adjacency); !ok {
		return out
	}

	return append(out, Objective{
		Kind:    KindCatch,
		Species: "porygon",
		Place:   place,
		Intent:  dexGameCornerIntent,
		Flee:    true,
		Note: fmt.Sprintf("(Game Corner prize: get Coin Case if needed, buy coins deterministically up to 9999, redeem PORYGON; worst-case cash Y%d)",
			skill.MaxPorygonCoinBudget()),
	})
}
