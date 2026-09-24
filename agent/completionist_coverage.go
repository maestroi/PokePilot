package agent

import "strings"

// completionistCoverageSignal is the thorough-player novelty policy. Every
// objective that reaches this function is already legal and offered by
// deterministic code; this layer favors meaningful optional content without
// turning Completionist into a test harness.
//
// The signal deliberately backs off while the party needs recovery. Safety and
// legality stay owned by the shared runtime. Low-value interaction churn belongs
// to Debug Coverage, not Completionist.
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
		}
		// Ordinary item/menu use is intentionally not a Completionist novelty
		// target. Debug Coverage owns exercising low-value interaction flows.
	}

	return s
}
