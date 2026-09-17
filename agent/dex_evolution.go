package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const (
	dexEvolutionLimit        = 6
	dexEvolutionSupplyIntent = "dex-evolution-supply"
	dexEvolutionStoneShop    = PlaceID("celadon mart 4f stones")
)

// appendDexEvolutionObjectives turns ROM-derived evolution sources into the
// existing deterministic training/item objectives. Repeatable bases needed by
// branched families are acquired first when no suitable individual is in the
// party. When a purchasable stone is the only missing immediate prerequisite,
// it offers a bounded supply purchase; the next observation then offers the
// verified item evolution. Virtual trade sources are appended from the same
// provider so trade-only species can become executable even when there are no
// remaining locally obtainable Dex targets.
func appendDexEvolutionObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	if len(obs.Dex.Targets) == 0 {
		return appendDexVirtualTradeObjectives(obs, out)
	}
	out = appendDexDuplicateEvolutionBaseObjectives(obs, known, out)
	if len(obs.Party) == 0 {
		return appendDexVirtualTradeObjectives(obs, out)
	}

	owned := pokedexOwnedSet(obs)
	seenObjective := map[string]bool{}
	for _, o := range out {
		seenObjective[o.String()] = true
	}

	added := 0
	for _, entry := range obs.Dex.Targets {
		if added >= dexEvolutionLimit || owned[entry.Species] {
			continue
		}
		for _, src := range entry.Sources {
			if added >= dexEvolutionLimit || src.From == "" {
				break
			}
			slot, mon, ok := partySpeciesSlot(obs.Party, src.From)
			if !ok {
				continue
			}

			var o Objective
			switch src.Kind {
			case AcquireLevelEvo:
				if !obs.HasGrass || mon.Level >= 100 || skill.BelowRetreatLine(mon.HP, mon.MaxHP) {
					continue
				}
				targetLevel := src.Level
				// If an evolution was cancelled at its normal threshold, the next
				// level-up is the next legitimate chance to trigger it.
				if targetLevel <= mon.Level {
					targetLevel = mon.Level + 1
				}
				if targetLevel == 0 || targetLevel > 100 {
					continue
				}
				o = Objective{
					Kind:    KindTrain,
					Species: src.From,
					Slot:    slot,
					Level:   targetLevel,
					Intent:  "dex-evolution",
					Note: fmt.Sprintf("(dex evolution: %s -> %s; verify the Pokédex-owned bit after level-up)",
						strings.ToUpper(string(src.From)), strings.ToUpper(string(entry.Species))),
				}
			case AcquireItemEvo:
				if src.Item == "" {
					continue
				}
				if bagItemQuantity(obs.Bag, src.Item) == 0 {
					if !dexEvolutionStonePurchaseAvailable(obs, known, src.Item) {
						continue
					}
					o = Objective{
						Kind:   KindBuy,
						Item:   src.Item,
						Qty:    1,
						Intent: dexEvolutionSupplyIntent,
						Note: fmt.Sprintf("(dex evolution supply: buy one %s for %s -> %s at Celadon Mart 4F)",
							strings.ToUpper(string(src.Item)), strings.ToUpper(string(src.From)), strings.ToUpper(string(entry.Species))),
					}
					break
				}
				o = Objective{
					Kind:   KindUseItem,
					Item:   src.Item,
					Slot:   slot,
					Intent: "dex-evolution",
					Note: fmt.Sprintf("(dex evolution: %s -> %s; preserve the evolution sequence and verify Pokédex ownership)",
						strings.ToUpper(string(src.From)), strings.ToUpper(string(entry.Species))),
				}
			default:
				continue
			}

			if seenObjective[o.String()] {
				continue
			}
			out = append(out, o)
			seenObjective[o.String()] = true
			added++
			// One executable source per missing target is enough for the
			// strategist; alternative routes remain visible in the catalog.
			break
		}
	}
	return appendDexVirtualTradeObjectives(obs, out)
}

func dexEvolutionStonePurchaseAvailable(obs Observation, known *Knowledge, item ItemID) bool {
	if !purchasableDexEvolutionStone(item) || len(obs.Bag) >= bagItemCapacity {
		return false
	}
	spec, ok := ItemEconomy(string(item))
	if !ok || spec.Category != InventoryEvolution || spec.UnitPrice == 0 {
		return false
	}
	ctx := EconomyContext(obs)
	if ctx == nil || ctx.SpendableMoney < spec.UnitPrice {
		return false
	}

	blocked := dexCatchBlockedPlaces(obs)
	hops := map[uint8]int{}
	var adjacency map[uint8][]uint8
	if known != nil {
		adjacency = known.nativeAdjacency()
		hops = mapHops(adjacency, obs.Map)
	}
	_, reachable := dexCatchPlaceDistance(obs, dexEvolutionStoneShop, blocked, hops, adjacency)
	return reachable
}

func purchasableDexEvolutionStone(item ItemID) bool {
	switch strings.ToLower(strings.TrimSpace(string(item))) {
	case "fire stone", "thunder stone", "water stone", "leaf stone":
		return true
	default:
		// Moon Stones are finite pickups in Red; never pretend a Mart can sell
		// them just because they are item-evolution inputs.
		return false
	}
}

func partySpeciesSlot(party []PartyMon, species SpeciesID) (int, PartyMon, bool) {
	for i, mon := range party {
		if mon.Species == species {
			return i, mon, true
		}
	}
	return 0, PartyMon{}, false
}

func bagItemQuantity(bag []Item, item ItemID) int {
	for _, entry := range bag {
		if strings.EqualFold(entry.Name, string(item)) {
			return entry.Quantity
		}
	}
	return 0
}
