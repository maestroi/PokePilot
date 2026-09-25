package agent

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
)

func TestAttachTravelResultProjectsTrainerDefeatAsCombatEvidence(t *testing.T) {
	result := ObjectiveResult{Objective: Objective{Kind: KindGoTo, Place: "route 3"}}
	attachTravelResult(&result, skill.TravelResult{BlackedOut: true, TrainerDefeat: true, Battles: 1})

	if result.Travel == nil || !result.Travel.BlackedOut || !result.Travel.TrainerDefeat {
		t.Fatalf("travel evidence = %+v, want trainer-defeat blackout", result.Travel)
	}
	if result.Battle == nil || result.Battle.Result != "lost" || result.Battle.Won {
		t.Fatalf("battle evidence = %+v, want portable lost combat evidence", result.Battle)
	}
	if result.Battle.Encounter != "" {
		t.Fatalf("travel trainer encounter = %q, want no invented identity", result.Battle.Encounter)
	}
}

func TestAttachTravelResultKeepsOrdinaryBlackoutOutOfCombatEvidence(t *testing.T) {
	result := ObjectiveResult{Objective: Objective{Kind: KindGoTo, Place: "route 3"}}
	attachTravelResult(&result, skill.TravelResult{BlackedOut: true})

	if result.Travel == nil || !result.Travel.BlackedOut || result.Travel.TrainerDefeat {
		t.Fatalf("travel evidence = %+v, want ordinary blackout", result.Travel)
	}
	if result.Battle != nil {
		t.Fatalf("ordinary blackout invented battle evidence: %+v", result.Battle)
	}
}

func TestTrainerBlackoutCompatibilityNormalizesAsGenericCombatDefeat(t *testing.T) {
	failure := normalizeRedFailure(
		gameruntime.FailurePhaseExecution,
		errors.Join(errors.New("wrapper"), skill.ErrTrainerBlackedOut),
		Observation{Controllable: true},
	)
	if failure.Cause != failureCauseCombatDefeat {
		t.Fatalf("cause = %q, want %q", failure.Cause, failureCauseCombatDefeat)
	}
	if failure.Class != gameruntime.FailureClassBlocked || !failure.Recoverable {
		t.Fatalf("failure = %+v, want recoverable blocked combat defeat", failure)
	}
}

func TestOrdinaryBlackoutStillNormalizesAsNavigationBlackout(t *testing.T) {
	failure := normalizeRedFailure(
		gameruntime.FailurePhaseExecution,
		skill.ErrBlackedOut,
		Observation{Controllable: true},
	)
	if failure.Cause != "blacked_out" {
		t.Fatalf("cause = %q, want ordinary blackout", failure.Cause)
	}
}
