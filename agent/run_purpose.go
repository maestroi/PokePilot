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
	return "RUN PURPOSE: DEBUG COVERAGE. Deliberately exercise NEW reachable game and runtime interactions to expose bugs, even when a normal player would skip them. Prefer unvisited maps/rooms, unseen NPC conversations, unbeaten trainers, uncollected pickups, new species/evolutions, shop/heal/menu/item/TM/HM flows and other newly offered interaction surfaces. Revisit areas when story state unlocks new interactions. Do not repeat already-covered work merely to churn actions, and do not manufacture failures: deterministic legality, safety and recovery remain authoritative. Treat story progression as an unlock step: while substantive NEW reachable coverage is already offered, exercise that frontier first; progress the story when the current frontier is exhausted or progression is required by the explicit run goal."
}

// ApplyRunPurpose turns Debug Coverage from a prompt hint into a bounded
// frontier policy. Legality still comes exclusively from the offered menu; this
// function only narrows already-legal choices while substantive new coverage is
// available. That matters for persistent strategic plans: without narrowing,
// the zero-call "single progression" continuation can race through story gates
// even while catches/NPCs/trainers/pickups are waiting in the same menu.
//
// A Dex terminal goal gets one additional rule: capture infrastructure and
// executable acquisition work come before unrelated coverage. This prevents a
// completion run from reaching several gyms with only its starter because the
// model kept choosing story progress over the deterministic Dex objectives.
func ApplyRunPurpose(obs Observation, offered []Objective, purpose, goal string) []Objective {
	annotated := AnnotateRunPurpose(obs, offered, purpose)
	if NormalizeRunPurpose(purpose) != RunPurposeDebugCoverage || len(annotated) == 0 {
		return annotated
	}

	// Recovery must remain authoritative. Do not hide a legal heal/retreat path
	// just because new coverage is also present.
	if partyHurt(obs) || leadOutOfPP(obs) {
		return annotated
	}

	if debugPurposeDexGoal(goal) {
		supply := filterPurposeObjectives(annotated, func(o Objective) bool {
			return debugDexCaptureSupply(obs, o)
		})
		if len(supply) == 0 && normalBallStock(obs) < minimumCaptureStock {
			// A direct buy exists only while standing in a Mart. For a Debug+Dex
			// run, add the same deterministic travel-and-buy recovery objective
			// used by the Red runtime when a reachable Mart is known. Keep this
			// scoped here so Champion/speedrun menus are not polluted by Dex-only
			// capture infrastructure.
			remote := restockCaptureObjectives(obs)
			annotated = append(annotated, AnnotateRunPurpose(obs, remote, purpose)...)
			supply = filterPurposeObjectives(annotated, func(o Objective) bool {
				return debugDexCaptureSupply(obs, o)
			})
		}
		if len(supply) > 0 {
			return supply
		}
		if acquisition := filterPurposeObjectives(annotated, func(o Objective) bool {
			return debugDexAcquisition(obs, o)
		}); len(acquisition) > 0 {
			return acquisition
		}
	}

	// High-value novelty is the current coverage frontier. Returning only that
	// frontier prevents story progression (and mundane travel) from shadowing
	// it. Once no substantive novelty remains, the full menu returns and story
	// progression can unlock the next frontier.
	const substantiveCoverage = 3.75
	if frontier := filterPurposeObjectives(annotated, func(o Objective) bool {
		priority, _ := debugCoveragePriority(obs, o)
		return priority >= substantiveCoverage
	}); len(frontier) > 0 {
		return frontier
	}
	return annotated
}

func debugPurposeDexGoal(raw string) bool {
	goal, structured, err := PlannerGoal(raw)
	return err == nil && structured && goal.Kind == GoalDex
}

func debugDexCaptureSupply(obs Observation, o Objective) bool {
	if len(obs.Dex.Targets) == 0 || normalBallStock(obs) >= minimumCaptureStock || o.Kind != KindBuy {
		return false
	}
	spec, ok := ItemEconomy(string(o.Item))
	return ok && spec.Category == InventoryCapture
}

func debugDexAcquisition(obs Observation, o Objective) bool {
	switch o.Kind {
	case KindCatch:
		return o.Species != "" && !pokedexOwnedSet(obs)[o.Species]
	case KindTrain, KindUseItem:
		return o.Intent == "dex-evolution"
	case KindBuy:
		return o.Intent == dexEvolutionSupplyIntent
	default:
		return false
	}
}

func filterPurposeObjectives(offered []Objective, keep func(Objective) bool) []Objective {
	out := make([]Objective, 0, len(offered))
	for _, objective := range offered {
		if keep(objective) {
			out = append(out, objective)
		}
	}
	return out
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
