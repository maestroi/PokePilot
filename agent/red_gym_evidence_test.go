package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestAttachGymBattleEvidencePreservesResolvedLossAcrossSettlementError(t *testing.T) {
	var result ObjectiveResult
	attachGymBattleEvidence(&result, "vermilion gym", state.ResultLost, errors.New("post-battle settlement failed"))

	if result.Battle == nil {
		t.Fatal("lost gym battle evidence was discarded because settlement also failed")
	}
	if result.Battle.Result != "lost" || result.Battle.Won {
		t.Fatalf("battle evidence = %+v, want resolved loss", result.Battle)
	}
	if result.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %q, want %q", result.Outcome, OutcomeBlocked)
	}
}

func TestAttachGymBattleEvidenceDoesNotInventWinForPreBattleError(t *testing.T) {
	var result ObjectiveResult
	attachGymBattleEvidence(&result, "vermilion gym", state.ResultWon, errors.New("leader battle never started"))

	if result.Battle != nil {
		t.Fatalf("pre-battle error invented zero-value win evidence: %+v", result.Battle)
	}
}

func TestAttachGymBattleEvidenceRecordsCleanWin(t *testing.T) {
	var result ObjectiveResult
	attachGymBattleEvidence(&result, "vermilion gym", state.ResultWon, nil)

	if result.Battle == nil || !result.Battle.Won || result.Battle.Result != "won" {
		t.Fatalf("battle evidence = %+v, want clean win", result.Battle)
	}
}
