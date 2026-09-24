package agent

// proactiveChallengePreparationObjective maps a structured readiness
// recommendation onto work that is already legal and offered this round. It
// never manufactures healing, shopping, training or travel objectives.
//
// Reactive post-loss combatPreparationObjective runs before this helper in Run;
// this path exists to avoid an unnecessary first loss at adapter-profiled major
// challenge boundaries.
func proactiveChallengePreparationObjective(
	obs Observation,
	offered []Objective,
	readiness []ChallengeReadiness,
	known *Knowledge,
) (Objective, ChallengeReadiness, bool) {
	for _, assessment := range readiness {
		if assessment.Action == ChallengeReady {
			continue
		}
		// Ordinary trainer telemetry with no adapter floor remains advisory.
		// Proactive interception is reserved for a known major boundary or a
		// typed prior loss (whose reactive path normally handles it first).
		if assessment.TargetReadiness == 0 && assessment.Losses == 0 {
			continue
		}
		if _, ok := resolveObjectiveKey(offered, assessment.Objective); !ok {
			continue
		}

		switch assessment.Action {
		case ChallengeHeal:
			for _, objective := range offered {
				if objective.Kind == KindHeal {
					return objective, assessment, true
				}
			}
			for _, objective := range offered {
				if objective.Kind == KindUseItem {
					return objective, assessment, true
				}
			}

		case ChallengeTrain:
			objective, ok, slotOnly := preferredTrainingObjective(offered, []ChallengeReadiness{assessment})
			if ok {
				return objective, assessment, true
			}
			if slotOnly {
				continue
			}
			if journey, _, ok := bestKnownTrainingJourney(obs, known, offered); ok {
				return journey, assessment, true
			}

		case ChallengeRestock:
			for _, objective := range offered {
				if objective.Kind != KindBuy {
					continue
				}
				name := string(objective.Item)
				if _, ok := hpHealingItems[name]; ok {
					return objective, assessment, true
				}
				if name == "revive" {
					return objective, assessment, true
				}
				if _, ok := fieldMedStatus[name]; ok {
					return objective, assessment, true
				}
				if _, ok := ppRestoreItems[name]; ok {
					return objective, assessment, true
				}
			}

		case ChallengeChangeParty:
			// Party composition is strategic: a random catch or arbitrary
			// reorder is not a safe deterministic substitute. Keep the
			// structured recommendation planner-visible and let the strategist
			// choose among the actual party-development objectives on the menu.
			continue
		}
	}
	return Objective{}, ChallengeReadiness{}, false
}
