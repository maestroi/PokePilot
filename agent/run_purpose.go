package agent

import (
	"fmt"
	"strings"
)

const (
	RunPurposeNormal        = "normal"
	RunPurposeDebugCoverage = "debug_coverage"
)

// NormalizeRunPurpose keeps empty/unknown values on the compatibility path.
// Purpose is orthogonal to play style: it changes why objectives are sampled,
// not their legality or the run's terminal goal.
func NormalizeRunPurpose(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case RunPurposeDebugCoverage, "debug-coverage", "debug", "coverage":
		return RunPurposeDebugCoverage
	case "", RunPurposeNormal:
		fallthrough
	default:
		return RunPurposeNormal
	}
}

// RunPurposeSystemNote adds the explicit testing intent only for debug runs.
// Normal runs keep the historical prompt unchanged.
func RunPurposeSystemNote(name string) string {
	if NormalizeRunPurpose(name) != RunPurposeDebugCoverage {
		return ""
	}
	return "RUN PURPOSE: DEBUG COVERAGE. Deliberately exercise NEW reachable game and runtime interactions to expose bugs, even when a normal player would skip them. Prefer unvisited maps/rooms, unseen NPC conversations, unbeaten trainers, uncollected pickups, new species/evolutions, shop/heal/menu/item/TM/HM flows and other newly offered interaction surfaces. Revisit areas when story state unlocks new interactions. Do not repeat already-covered work merely to churn actions, and do not manufacture failures: deterministic legality, safety and recovery remain authoritative. Progress the story whenever it unlocks additional coverage or is required by the explicit run goal."
}

// AnnotateRunPurpose makes the debug intent inspectable in the same objective
// menu the planner already sees. Objective generation remains the sole source
// of truth for legality; this layer only highlights likely-new surfaces.
func AnnotateRunPurpose(obs Observation, offered []Objective, purpose string) []Objective {
	if NormalizeRunPurpose(purpose) != RunPurposeDebugCoverage {
		return offered
	}
	out := append([]Objective(nil), offered...)
	for i := range out {
		bonus, tag := debugCoveragePriority(obs, out[i])
		if bonus <= 0 || tag == "" {
			continue
		}
		// Recovery still matters in a debug run. Keep coverage attractive while
		// injured, but do not make a novelty hint overpower an urgent heal.
		if partyHurt(obs) || leadOutOfPP(obs) {
			bonus *= 0.25
		}
		annotation := fmt.Sprintf("[debug-coverage %.2f: %s]", bonus, tag)
		if out[i].Note == "" {
			out[i].Note = annotation
		} else {
			out[i].Note += " " + annotation
		}
	}
	return out
}

func debugCoveragePriority(obs Observation, o Objective) (float64, string) {
	switch o.Kind {
	case KindGoTo:
		if strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map") {
			return 5.0, "new-map"
		}
	case KindTalk:
		// Talk objectives exclude coordinates already recorded by Knowledge.
		return 5.0, "new-npc"
	case KindTrainer:
		// Offered trainer objectives are still challengeable.
		return 4.5, "new-trainer"
	case KindPickup:
		return 4.5, "new-pickup"
	case KindCatch:
		if o.Species != "" && !pokedexOwnedSet(obs)[o.Species] {
			return 4.25, "new-species"
		}
	case KindTrain:
		if o.Intent == "dex-evolution" {
			return 4.0, "new-evolution"
		}
	case KindUseItem:
		item := strings.ToLower(strings.TrimSpace(string(o.Item)))
		if strings.HasPrefix(item, "tm") || strings.HasPrefix(item, "hm") {
			return 3.75, "machine-use"
		}
		return 1.5, "item-flow"
	case KindBuy:
		return 1.75, "shop-flow"
	case KindHeal:
		return 1.0, "heal-flow"
	case KindProgress, KindGym:
		return 0.75, "unlock-coverage"
	}
	return 0, ""
}
