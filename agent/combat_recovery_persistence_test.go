package agent

import (
	"errors"
	"fmt"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func assertNoLegacyCombatWriterModes(t *testing.T, known *Knowledge) {
	t.Helper()
	for storage := range known.Failures {
		_, mode, ok := parseFailureStorageKey(storage)
		if !ok {
			continue
		}
		switch mode {
		case legacyFailureModeTrainerLoss, legacyFailureModeGymLoss, legacyFailureModeGymRetry:
			t.Fatalf("new recovery state wrote legacy mode %q: %s", mode, storage)
		}
	}
}

func TestNewCombatRecoveryStateUsesOnlyGenericModes(t *testing.T) {
	cases := []struct {
		name   string
		obj    Objective
		record func(*Knowledge)
	}{
		{
			name: "required gym direct caller",
			obj:  Objective{Kind: KindGym, Place: "pewter gym"},
			record: func(k *Knowledge) {
				gym := Objective{Kind: KindGym, Place: "pewter gym"}
				k.Failed(gym, gymOutcomeErr(gym, state.ResultLost))
			},
		},
		{
			name: "legacy trainer sentinel direct caller",
			obj:  Objective{Kind: KindGoTo, Place: "route 3", Flee: true},
			record: func(k *Knowledge) {
				o := Objective{Kind: KindGoTo, Place: "route 3", Flee: true}
				k.Failed(o, fmt.Errorf("wrapped: %w", skill.ErrTrainerBlackedOut))
			},
		},
		{
			name: "live structured combat result",
			obj:  Objective{Kind: KindProgress, Progress: "volcano_badge"},
			record: func(k *Knowledge) {
				o := Objective{Kind: KindProgress, Progress: "volcano_badge"}
				k.FailedResult(ObjectiveResult{
					Objective: o,
					Outcome:   OutcomeBlocked,
					Battle:    &BattleEvidence{Encounter: "gym:blaine", Result: "lost"},
					Failure: &gameruntime.Failure{
						Class:       gameruntime.FailureClassBlocked,
						Cause:       failureCauseCombatDefeat,
						Recoverable: true,
					},
				}, errors.New("diagnostic"))
			},
		},
		{
			name: "live legacy trainer cause compatibility",
			obj:  Objective{Kind: KindTrainer, Location: "route-3", X: 10, Y: 6},
			record: func(k *Knowledge) {
				o := Objective{Kind: KindTrainer, Location: "route-3", X: 10, Y: 6}
				k.FailedResult(ObjectiveResult{
					Objective: o,
					Outcome:   OutcomeBlocked,
					Failure: &gameruntime.Failure{
						Class:       gameruntime.FailureClassBlocked,
						Cause:       "trainer_blacked_out",
						Recoverable: true,
					},
				}, errors.New("diagnostic"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			known := NewKnowledge(nil)
			tc.record(known)
			assertNoLegacyCombatWriterModes(t, known)
			if !combatLossRecorded(known, tc.obj) {
				t.Fatalf("new combat loss was not stored generically: %+v", known.Failures)
			}

			known.releaseCombatLossGates()
			assertNoLegacyCombatWriterModes(t, known)
			if combatLossRecorded(known, tc.obj) {
				t.Fatalf("loss gate remained after readiness progress: %+v", known.Failures)
			}
			if !combatRetryKeys(known)[combatRecoveryObjective(tc.obj).Key()] {
				t.Fatalf("generic combat retry was not scheduled: %+v", known.Failures)
			}
		})
	}
}

func TestLegacyCombatModesRemainReadableAndMigrateToGenericRetry(t *testing.T) {
	known := NewKnowledge(nil)
	route := Objective{Kind: KindGoTo, Place: "route 3"}
	gym := Objective{Kind: KindGym, Place: "pewter gym"}

	known.Failures[legacyTrainerLossStorageKey(route)] = Failure{Objective: route.String(), Times: 2, Last: "legacy trainer loss"}
	known.Failures[legacyGymLossStorageKey(string(gym.Place))] = Failure{Objective: gym.String(), Times: 3, Last: "legacy gym loss"}

	if !combatLossRecorded(known, route) || !combatLossRecorded(known, gym) {
		t.Fatalf("legacy combat modes were not readable: %+v", known.Failures)
	}

	known.releaseCombatLossGates()
	assertNoLegacyCombatWriterModes(t, known)

	ready := combatRetryKeys(known)
	if !ready[combatRecoveryObjective(route).Key()] || !ready[combatRecoveryObjective(gym).Key()] {
		t.Fatalf("legacy losses did not migrate to generic retry state: %+v", known.Failures)
	}
}
