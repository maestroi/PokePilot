package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

const dexEvolutionLimit = 6

// appendDexEvolutionObjectives turns ROM-derived evolution sources into the
// existing deterministic training/item objectives. The executor owns the
// mechanics; this layer only offers evolutions whose base Pokemon is currently
// in the party and whose immediate prerequisite is available.
func appendDexEvolutionObjectives(obs Observation, out []Objective) []Objective {
	if len(obs.Dex.Targets) == 0 || len(obs.Party) == 0 {
		return out
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
				if src.Item == "" || bagItemQuantity(obs.Bag, src.Item) == 0 {
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
	return out
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
