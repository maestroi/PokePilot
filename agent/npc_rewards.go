package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const redNPCRewardIntent = "npc-choice-reward"

const (
	pewterGymMapID       uint8 = 0x36
	pewterGymGuideHomeX  uint8 = 7
	pewterGymGuideHomeY  uint8 = 10
)

// appendRedNPCRewardObjectives turns Red's known YES/NO item handoffs into
// semantic collect objectives. A reward on the current map reuses KindPickup's
// positive bag-delta postcondition; a reachable remote reward first exposes an
// interaction-owned journey, then the collect objective appears next round.
//
// This makes the rods discoverable to Completionist/Dex play instead of
// waiting for random generic conversation, and only offers Oak's aides once
// their ROM Pokedex-owned thresholds are already satisfied.
func appendRedNPCRewardObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	blocked := dexCatchBlockedPlaces(obs)
	var (
		hops      map[uint8]int
		adjacency map[uint8][]uint8
	)
	if known != nil {
		adjacency = known.Adjacency
		hops = mapHops(adjacency, obs.Map)
	}

	for _, reward := range skill.ChoiceRewards() {
		item := ItemID(reward.ItemName)
		if bagItemQuantity(obs.Bag, item) > 0 {
			continue
		}
		if reward.MinOwned > 0 && len(obs.PokedexOwned) < reward.MinOwned {
			continue
		}

		note := fmt.Sprintf("(scripted NPC reward: receive %s; YES choice and bag-space handling are deterministic)",
			strings.ToUpper(reward.ItemName))
		if reward.MinOwned > 0 {
			note = fmt.Sprintf("(Oak's aide reward: receive %s after owning %d Pokemon; YES choice and bag-space handling are deterministic)",
				strings.ToUpper(reward.ItemName), reward.MinOwned)
		}

		if obs.Map == reward.Map {
			out = append(out, Objective{
				Kind:   KindPickup,
				X:      reward.X,
				Y:      reward.Y,
				Item:   item,
				Intent: redNPCRewardIntent,
				Note:   note,
			})
			continue
		}

		// Cross-map reward routing depends on the learned/static map adjacency
		// carried by Knowledge. With no graph evidence, do not manufacture a
		// remote journey merely because the interaction destination is known.
		if known == nil {
			continue
		}
		place := PlaceID(reward.Place)
		if _, ok := dexCatchPlaceDistance(obs, place, blocked, hops, adjacency); !ok {
			continue
		}
		out = append(out, Objective{
			Kind:   KindGoTo,
			Place:  place,
			Flee:   true,
			Intent: redNPCRewardIntent,
			Note:   note,
		})
	}
	return out
}

// redChoiceInteractionActor is the adapter-owned classification for custom
// text_asm people that generic Talk must not drive. Item reward actors are
// handled by appendRedNPCRewardObjectives. Pewter's gym guide is a harmless
// flavor choice, but it still needs an explicitly-owned choice verb before we
// can converse with it safely, so generic Talk suppresses it for now.
func redChoiceInteractionActor(mapID, x, y uint8) bool {
	if skill.IsChoiceRewardActor(mapID, x, y) {
		return true
	}
	return mapID == pewterGymMapID && x == pewterGymGuideHomeX && y == pewterGymGuideHomeY
}
