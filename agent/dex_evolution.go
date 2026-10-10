package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const (
	dexEvolutionLimit        = 6
	dexEvolutionSupplyIntent = "dex-evolution-supply"
	// combatRecoverySupplyIntent marks a healing purchase that travels to the
	// nearest reachable shop stocking the item before buying.
	combatRecoverySupplyIntent = "combat-recovery-supply"
	dexEvolutionStoneShop      = PlaceID("celadon mart 4f stones")
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
	entries := dexEvolutionCandidateEntries(obs)
	if len(entries) == 0 {
		return appendDexVirtualTradeObjectives(obs, known, out)
	}
	out = appendDexDuplicateEvolutionBaseObjectives(obs, known, out)
	if len(obs.Party) == 0 {
		return appendDexVirtualTradeObjectives(obs, known, out)
	}

	owned := pokedexOwnedSet(obs)
	seenObjective := map[string]bool{}
	for _, o := range out {
		seenObjective[o.String()] = true
	}

	added := 0
	for _, entry := range entries {
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
					// appendDexStoneStockObjectives buys every stone the Dex
					// still needs in one trip.
					continue
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
	return appendDexVirtualTradeObjectives(obs, known, appendDexStoneStockObjectives(obs, known, out))
}

// appendDexStoneStockObjectives offers one bulk purchase per stone kind: the
// number of unowned Dex targets that evolve with it, minus stones held,
// bounded by spendable money. Stone need is fixed by the ROM evolution data,
// not by where the base Pokémon currently is, so a run can stock every stone
// in its first Celadon trip instead of returning once per evolution.
func appendDexStoneStockObjectives(obs Observation, known *Knowledge, out []Objective) []Objective {
	owned := pokedexOwnedSet(obs)
	need := map[ItemID]int{}
	var order []ItemID
	for _, entry := range obs.Dex.Targets {
		if owned[entry.Species] {
			continue
		}
		for _, src := range entry.Sources {
			if src.Kind != AcquireItemEvo || !purchasableDexEvolutionStone(src.Item) {
				continue
			}
			if need[src.Item] == 0 {
				order = append(order, src.Item)
			}
			need[src.Item]++
			break
		}
	}
	for _, item := range order {
		want := need[item] - bagItemQuantity(obs.Bag, item)
		if want <= 0 || !dexEvolutionStonePurchaseAvailable(obs, known, item) {
			continue
		}
		spec, _ := ItemEconomy(string(item))
		qty := boundedAffordableQty(want, EconomyContext(obs).SpendableMoney, spec.UnitPrice)
		if qty <= 0 {
			continue
		}
		out = append(out, Objective{
			Kind:   KindBuy,
			Item:   item,
			Qty:    qty,
			Intent: dexEvolutionSupplyIntent,
			Note: fmt.Sprintf("(dex evolution supply: stock %d %s for %d unowned evolution(s) at Celadon Mart 4F)",
				qty, strings.ToUpper(string(item)), need[item]),
		})
	}
	return out
}

func dexEvolutionCandidateEntries(obs Observation) []DexEntry {
	out := append([]DexEntry(nil), obs.Dex.Targets...)
	seen := make(map[SpeciesID]bool, len(out))
	for _, entry := range out {
		seen[entry.Species] = true
	}
	for _, entry := range obs.Dex.Unavailable {
		if entry.Species == "" || seen[entry.Species] {
			continue
		}
		if entry.Unavailable != UnavailableNoLocalSource &&
			!strings.HasPrefix(entry.Unavailable, UnavailableForfeited+":") {
			continue
		}
		for _, src := range entry.Sources {
			if src.From == "" || (src.Kind != AcquireLevelEvo && src.Kind != AcquireItemEvo) {
				continue
			}
			if _, _, ok := partySpeciesSlot(obs.Party, src.From); !ok {
				continue
			}
			out = append(out, entry)
			seen[entry.Species] = true
			break
		}
	}
	return out
}

func dexEvolutionStonePurchaseAvailable(obs Observation, known *Knowledge, item ItemID) bool {
	// BuyEvolutionStone frees a retired slot itself (EnsureBagSpaceFor).
	if !purchasableDexEvolutionStone(item) ||
		(bagItemQuantity(obs.Bag, item) == 0 && len(obs.Bag)-obs.ReclaimableBagSlots >= bagItemCapacity) {
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
	hops := map[uint16]int{}
	var adjacency map[uint16][]uint16
	if known != nil {
		adjacency = known.nativeAdjacency()
		hops = mapHops(adjacency, uint16(obs.Map))
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
