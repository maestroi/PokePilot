package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const redNPCRewardIntent = "npc-choice-reward"

const (
	pewterGymMapID      uint8 = 0x36
	pewterGymGuideHomeX uint8 = 7
	pewterGymGuideHomeY uint8 = 10
)

// redCustomChoiceActors are Red text_asm/map-script actors whose interaction
// can open a gameplay choice or stateful service that generic KindTalk must not
// own. Dedicated semantic paths already own several of these transactions
// (Safari sessions, Fuchsia progression, Dex trades); the rest are suppressed
// until they gain an explicit verb. Keeping the table at the Red adapter seam
// makes the safety rule auditable without teaching generic Talk game-specific
// answers.
var redCustomChoiceActors = []struct {
	mapID uint8
	x     uint8
	y     uint8
}{
	{mapID: 0x04, x: 15, y: 9}, // Lavender little girl: flavor YES/NO.
	{mapID: pewterGymMapID, x: pewterGymGuideHomeX, y: pewterGymGuideHomeY}, // Pewter Gym guide: flavor YES/NO.
	{mapID: 0x48, x: 2, y: 3},  // Daycare gentleman: deposit/withdraw party member and money.
	{mapID: 0x9B, x: 2, y: 3},  // Warden: Gold Teeth/Fuchsia progression state machine.
	{mapID: 0x9C, x: 6, y: 2},  // Safari gate worker: paid Safari session lifecycle.
	{mapID: 0x9C, x: 1, y: 4},  // Safari gate worker: flavor YES/NO explanation.
	{mapID: 0xAA, x: 5, y: 2},  // Cinnabar scientist: fossil revival transaction.
	{mapID: 0xAA, x: 7, y: 6},  // Cinnabar scientist: in-game Pokemon trade.
	{mapID: 0xE5, x: 5, y: 3},  // Name Rater: rename service/menu.
}

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
// text_asm/map-script people that generic Talk must not drive. Item reward
// actors are handled by appendRedNPCRewardObjectives; the explicit table above
// covers remaining stateful services and flavor choices until their semantic
// owner intentionally drives them.
func redChoiceInteractionActor(mapID, x, y uint8) bool {
	if skill.IsChoiceRewardActor(mapID, x, y) {
		return true
	}
	for _, actor := range redCustomChoiceActors {
		if actor.mapID == mapID && actor.x == x && actor.y == y {
			return true
		}
	}
	return false
}
