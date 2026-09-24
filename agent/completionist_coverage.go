package agent

import (
	"strconv"
	"strings"
)

// completionistCoverageSignal turns Completionist from a broad preference for
// optional work into an explicit novelty/coverage policy. Every objective that
// reaches this function is already legal and offered by deterministic code;
// this layer only makes novel game surfaces more attractive to the planner.
//
// The signal deliberately backs off while the party needs recovery. Safety and
// legality stay owned by the shared runtime, while Completionist spends healthy
// states probing optional maps, NPCs, trainers, items, machines and Dex paths.
func completionistCoverageSignal(obs Observation, o Objective, profile PlayStyleProfile) NaturalPlaySignal {
	if profile.Name != PlayStyleCompletionist {
		return NaturalPlaySignal{}
	}

	var s NaturalPlaySignal
	add := func(tag string, bonus float64) {
		if bonus <= 0 {
			return
		}
		// Completionist already has a larger NaturalPlayScale. Reuse it so the
		// coverage layer remains data-driven instead of hard-coding a second
		// profile strength.
		bonus *= profile.NaturalPlayScale
		if partyHurt(obs) || leadOutOfPP(obs) {
			bonus *= 0.20
		}
		s.Tags = appendNaturalTag(s.Tags, tag)
		s.Bonus += bonus
	}

	switch o.Kind {
	case KindGoTo:
		if strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map") {
			add("coverage-new-map", 0.58)
		}

	case KindTalk:
		// Offer removes already-talked-to coordinates, so an offered talk is a
		// genuine new NPC interaction for this run.
		add("coverage-new-npc", 0.46)

	case KindTrainer:
		// Trainer objectives are exposed only while the trainer is still
		// challengeable. Defeating them expands battle/AI/map coverage.
		add("coverage-new-trainer", 0.42)

	case KindPickup:
		// A present pickup is durable new interaction coverage and often opens a
		// second path later (TM/HM use, healing, key items, economy).
		add("coverage-new-item", 0.48)

	case KindCatch:
		if o.Species != "" && !pokedexOwnedSet(obs)[o.Species] {
			add("coverage-new-species", 0.62)
		}

	case KindTrain:
		if o.Intent == "dex-evolution" {
			add("coverage-evolution", 0.58)
		}

	case KindUseItem:
		item := strings.ToLower(string(o.Item))
		if strings.HasPrefix(item, "tm") || strings.HasPrefix(item, "hm") {
			add("coverage-machine-use", 0.48)
		} else {
			// Using ordinary inventory still exercises deterministic menu/item
			// behavior, but do not let potion churn compete with genuinely novel
			// surfaces.
			add("coverage-item-use", 0.08)
		}
	}

	return s
}


// Run purpose is orthogonal to play style and terminal goal. Empty/normal
// preserves player-facing behavior; debug_coverage deliberately maximizes
// novel reachable interaction coverage to expose bugs.
const (
	RunPurposeNormal        = "normal"
	RunPurposeDebugCoverage = "debug_coverage"
)

func NormalizeRunPurpose(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case RunPurposeDebugCoverage, "debug", "coverage", "debug-coverage":
		return RunPurposeDebugCoverage
	default:
		return RunPurposeNormal
	}
}

// debugCoverageSignal is intentionally stronger and broader than the
// Completionist signal. Completionist behaves like a thorough player; debug
// coverage is allowed to value otherwise low-payoff interactions because its
// purpose is exercising the runtime and game surfaces.
func debugCoverageSignal(obs Observation, o Objective) NaturalPlaySignal {
	var s NaturalPlaySignal
	add := func(tag string, bonus float64) {
		if bonus <= 0 {
			return
		}
		if partyHurt(obs) || leadOutOfPP(obs) {
			bonus *= 0.25
		}
		s.Tags = appendNaturalTag(s.Tags, tag)
		s.Bonus += bonus
	}

	switch o.Kind {
	case KindGoTo:
		if strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map") {
			add("debug-new-map", 1.45)
		}
	case KindTalk:
		add("debug-new-npc", 1.30)
	case KindTrainer:
		add("debug-new-trainer", 1.20)
	case KindPickup:
		add("debug-new-item", 1.25)
	case KindCatch:
		if o.Species != "" && !pokedexOwnedSet(obs)[o.Species] {
			add("debug-new-species", 1.35)
		}
	case KindTrain:
		if o.Intent == "dex-evolution" {
			add("debug-evolution", 1.25)
		}
	case KindUseItem:
		item := strings.ToLower(string(o.Item))
		if strings.HasPrefix(item, "tm") || strings.HasPrefix(item, "hm") {
			add("debug-machine-use", 1.10)
		} else {
			// Ordinary item flows are worth exercising in debug runs even when
			// they would be noise for a normal Completionist.
			add("debug-item-use", 0.35)
		}
	case KindBuy:
		add("debug-shop-flow", 0.45)
	case KindHeal:
		if partyHurt(obs) || leadOutOfPP(obs) {
			add("debug-recovery-flow", 0.30)
		}
	}
	return s
}

// AnnotateRunPurpose layers run-purpose priorities over the already legal
// objective menu. It does not create objectives or bypass deterministic
// legality. Debug coverage is therefore safe to combine with any play style or
// terminal goal.
func AnnotateRunPurpose(obs Observation, offered []Objective, purpose string) []Objective {
	if NormalizeRunPurpose(purpose) != RunPurposeDebugCoverage {
		return offered
	}
	out := append([]Objective(nil), offered...)
	for i := range out {
		signal := debugCoverageSignal(obs, out[i])
		if signal.Bonus <= 0 {
			continue
		}
		annotation := "[debug_coverage +" + fmtFloat(signal.Bonus) + ": " + strings.Join(signal.Tags, "+") + "]"
		if out[i].Note == "" {
			out[i].Note = annotation
		} else {
			out[i].Note += " " + annotation
		}
	}
	return out
}

func fmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
