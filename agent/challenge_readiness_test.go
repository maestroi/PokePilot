package agent

import "testing"

func readinessTestChallenge() Objective {
	return Objective{Kind: KindGym, Place: "test gym"}
}

func readinessTestObservation(level uint8) Observation {
	return Observation{
		PartyCount: 1,
		Party: []PartyMon{{
			Species: "testmon", Level: level, HP: 50, MaxHP: 50,
		}},
		LeadMoves: []Move{{Power: 40, Type: "normal"}},
	}
}

func TestEvaluateChallengeReadinessReady(t *testing.T) {
	obs := readinessTestObservation(20)
	got := EvaluateChallengeReadiness(obs, NewKnowledge(nil), readinessTestChallenge(), ChallengeReadinessProfile{})
	if got.Action != ChallengeReady {
		t.Fatalf("action = %q, want ready: %+v", got.Action, got)
	}
	if got.CurrentReadiness != 80 {
		t.Fatalf("current readiness = %d, want 80", got.CurrentReadiness)
	}
}

func TestEvaluateChallengeReadinessHealsBeforeTraining(t *testing.T) {
	obs := readinessTestObservation(20)
	obs.Party[0].HP = 10
	obs.RecoveryCheckpoint = "test pokemon center"

	known := NewKnowledge(nil)
	challenge := readinessTestChallenge()
	known.Failures[combatLossFailureKey(challenge)] = Failure{
		Objective: challenge.String(), Times: 1, ReadinessTarget: 120,
	}

	got := EvaluateChallengeReadiness(obs, known, challenge, ChallengeReadinessProfile{})
	if got.Action != ChallengeHeal {
		t.Fatalf("action = %q, want heal before training: %+v", got.Action, got)
	}
	if !got.RecoveryAvailable {
		t.Fatalf("recovery should be available: %+v", got)
	}
}

func TestEvaluateChallengeReadinessTrainsBelowLossTarget(t *testing.T) {
	obs := readinessTestObservation(20)
	known := NewKnowledge(nil)
	challenge := readinessTestChallenge()
	known.Failures[combatLossFailureKey(challenge)] = Failure{
		Objective: challenge.String(), Times: 2, ReadinessBaseline: 80, ReadinessTarget: 112,
	}

	got := EvaluateChallengeReadiness(obs, known, challenge, ChallengeReadinessProfile{})
	if got.Action != ChallengeTrain {
		t.Fatalf("action = %q, want train: %+v", got.Action, got)
	}
	if got.TargetReadiness != 112 || got.Losses != 2 {
		t.Fatalf("preparation evidence = %+v, want target 112 after two losses", got)
	}
}

func TestEvaluateChallengeReadinessRestocksAfterLossWithNoHealingStock(t *testing.T) {
	obs := readinessTestObservation(20)
	obs.MartStock = []string{"super potion"}
	known := NewKnowledge(nil)
	challenge := readinessTestChallenge()
	known.Failures[combatLossFailureKey(challenge)] = Failure{
		Objective: challenge.String(), Times: 1, ReadinessBaseline: 80, ReadinessTarget: 80,
	}

	got := EvaluateChallengeReadiness(obs, known, challenge, ChallengeReadinessProfile{})
	if got.Action != ChallengeRestock {
		t.Fatalf("action = %q, want restock: %+v", got.Action, got)
	}
	if got.EmergencyHeals != 0 {
		t.Fatalf("emergency heals = %d, want 0", got.EmergencyHeals)
	}
}

func TestEvaluateChallengeReadinessChangesUndersizedParty(t *testing.T) {
	obs := readinessTestObservation(20)
	got := EvaluateChallengeReadiness(obs, NewKnowledge(nil), readinessTestChallenge(), ChallengeReadinessProfile{
		MinimumUsableMons: 2,
	})
	if got.Action != ChallengeChangeParty {
		t.Fatalf("action = %q, want change_party: %+v", got.Action, got)
	}
}

func TestEvaluateChallengeReadinessUsesKnownMoveCoverage(t *testing.T) {
	obs := readinessTestObservation(20)
	got := EvaluateChallengeReadiness(obs, NewKnowledge(nil), readinessTestChallenge(), ChallengeReadinessProfile{
		PreferredMoveTypes: []string{"water", "grass"},
	})
	if got.Action != ChallengeChangeParty {
		t.Fatalf("action = %q, want change_party for known coverage gap: %+v", got.Action, got)
	}

	obs.LeadMoves = []Move{{Power: 40, Type: "water"}}
	got = EvaluateChallengeReadiness(obs, NewKnowledge(nil), readinessTestChallenge(), ChallengeReadinessProfile{
		PreferredMoveTypes: []string{"water", "grass"},
	})
	if got.Action != ChallengeReady {
		t.Fatalf("action = %q, want ready with preferred coverage: %+v", got.Action, got)
	}
}

func TestRepeatedCombatLossesRaiseReadinessTarget(t *testing.T) {
	obs := readinessTestObservation(20)
	one := Failure{Times: 1}
	three := Failure{Times: 3}
	stampCombatPreparation(&one, obs)
	stampCombatPreparation(&three, obs)
	if three.ReadinessTarget <= one.ReadinessTarget {
		t.Fatalf("three-loss target %d did not exceed one-loss target %d", three.ReadinessTarget, one.ReadinessTarget)
	}
}

func TestObjectiveOfferCarriesStructuredChallengeReadiness(t *testing.T) {
	obs := readinessTestObservation(20)
	obs.Catalog = ObjectiveCatalog{Challenges: []CatalogChallenge{{
		Place: "test gym",
		Readiness: ChallengeReadinessProfile{MinimumReadiness: 100},
	}}}

	offer := OfferWithEvidence(obs, NewKnowledge(nil))
	if len(offer.Readiness) != 1 {
		t.Fatalf("readiness count = %d, want 1: %+v", len(offer.Readiness), offer.Readiness)
	}
	got := offer.Readiness[0]
	if got.Action != ChallengeTrain || got.TargetReadiness != 100 {
		t.Fatalf("offer readiness = %+v, want train toward 100", got)
	}
}
