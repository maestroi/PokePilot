package agent

import (
	"sort"
	"strings"
)

const dexCleanupNote = "(dex cleanup: deterministic post-Hall-of-Fame choice from currently executable collection actions)"

type dexCleanupCandidate struct {
	Objective Objective
	Cost      int
	TargetDex uint8
}

// ApplyDexCleanupPolicy narrows a post-Hall-of-Fame Dex run to one bounded,
// already-legal collection action. It never creates objectives or bypasses
// prerequisites: when no executable Dex action is currently offered, the menu
// is left untouched so ordinary planning can obtain balls, rods, money, field
// moves, storage space, or any other prerequisite first.
//
// This is deliberately a postgame policy. Normal story play remains free to
// make opportunistic catches, while cleanup becomes reproducible once the main
// campaign is complete and the deterministic Dex goal is the work keeping the
// run alive.
func ApplyDexCleanupPolicy(obs Observation, offered []Objective) []Objective {
	if !obs.Story.Has(ProgressMainStoryComplete) || len(obs.Dex.Targets) == 0 || len(offered) == 0 {
		return offered
	}

	// Do not force a collection battle through an obviously worn-down party.
	// If a local heal is already legal, make recovery deterministic too. If it
	// is not, leave the full menu alone so normal route/recovery planning can
	// reach a Center instead of hiding that prerequisite.
	if partyHurt(obs) || leadOutOfPP(obs) {
		for _, o := range offered {
			if o.Kind == KindHeal {
				o = appendObjectiveNote(o, dexCleanupNote)
				return []Objective{o}
			}
		return offered
	}

	candidates := make([]dexCleanupCandidate, 0, len(offered))
	for _, o := range offered {
		cost, targetDex, ok := dexCleanupObjectiveCost(obs, o)
		if !ok {
			continue
		}
		candidates = append(candidates, dexCleanupCandidate{Objective: o, Cost: cost, TargetDex: targetDex})
	}
	if len(candidates) == 0 {
		return offered
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Cost != candidates[j].Cost {
			return candidates[i].Cost < candidates[j].Cost
		}
		if candidates[i].TargetDex != candidates[j].TargetDex {
			return candidates[i].TargetDex < candidates[j].TargetDex
		}
		return candidates[i].Objective.String() < candidates[j].Objective.String()
	})
	chosen := appendObjectiveNote(candidates[0].Objective, dexCleanupNote)
	return []Objective{chosen}
}

func dexCleanupObjectiveCost(obs Observation, o Objective) (int, uint8, bool) {
	target, dex, targetOK := dexCleanupTarget(obs, o)
	_ = target

	switch o.Kind {
	case KindUseItem:
		if o.Intent == "dex-evolution" && targetOK {
			return 5, dex, true
		}
	case KindBuy:
		if o.Intent == dexEvolutionSupplyIntent && targetOK {
			return 15, dex, true
		}
	case KindTrain:
		if o.Intent != "dex-evolution" || !targetOK {
			return 0, 0, false
		}
		levels := int(o.Level)
		if o.Slot >= 0 && o.Slot < len(obs.Party) {
			levels -= int(obs.Party[o.Slot].Level)
		}
		if levels < 1 {
			levels = 1
		}
		// One or two nearby levels can beat a remote hunt; long grinding loses
		// to a supported direct capture route when both are executable.
		return 18 + levels*2, dex, true
	case KindCatch:
		duplicateBase := strings.Contains(strings.ToLower(o.Note), "dex duplicate base")
		if !targetOK && !duplicateBase {
			return 0, 0, false
		}
		if duplicateBase && !targetOK {
			dex = 255
		}
		sameMap := o.Place == "" || o.Place == obs.Location || o.Place == mapPlace(obs.Map)
		switch o.Intent {
		case dexGiftIntent:
			return 8, dex, true
		case dexTradeIntent, dexFossilIntent:
			return 10, dex, true
		case dexGameCornerIntent:
			return 12, dex, true
		case dexStaticIntent:
			return 14, dex, true
		case dexFishingIntent, dexWaterIntent:
			if sameMap {
				return 18, dex, true
			}
			return 30, dex, true
		case dexSafariIntent:
			return 34, dex, true
		default:
			if duplicateBase {
				if sameMap {
					return 20, dex, true
				}
				return 32, dex, true
			}
			if sameMap {
				return 15, dex, true
			}
			return 28, dex, true
		}
	}
	return 0, 0, false
}

func dexCleanupTarget(obs Observation, o Objective) (SpeciesID, uint8, bool) {
	for _, entry := range obs.Dex.Targets {
		if o.Kind == KindCatch && entry.Species == o.Species {
			return entry.Species, entry.Dex, true
		}
		for _, src := range entry.Sources {
			switch o.Kind {
			case KindTrain:
				if o.Intent == "dex-evolution" && src.Kind == AcquireLevelEvo && src.From == o.Species && (src.Level == 0 || o.Level >= src.Level) {
					return entry.Species, entry.Dex, true
				}
			case KindUseItem:
				if o.Intent != "dex-evolution" || src.Kind != AcquireItemEvo || !strings.EqualFold(string(src.Item), string(o.Item)) {
					continue
				}
				if o.Slot >= 0 && o.Slot < len(obs.Party) && src.From == obs.Party[o.Slot].Species {
					return entry.Species, entry.Dex, true
				}
			case KindBuy:
				if o.Intent != dexEvolutionSupplyIntent || src.Kind != AcquireItemEvo || !strings.EqualFold(string(src.Item), string(o.Item)) {
					continue
				}
				if _, _, ok := partySpeciesSlot(obs.Party, src.From); ok {
					return entry.Species, entry.Dex, true
				}
			}
		}
	}
	return "", 0, false
}
