package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func TestGymLossClassificationDoesNotDependOnErrorProse(t *testing.T) {
	gym := Objective{Kind: KindGym, Place: "pewter gym"}

	proseOnly := NewKnowledge(nil)
	proseOnly.Failed(gym, errors.New("lost to the gym leader (blacked out to the center)"))
	if combatLossRecorded(proseOnly, gym) {
		t.Fatal("plain error prose was classified as a gym loss")
	}
	if got := proseOnly.Failures[objectiveStorageKey(gym)].Times; got != 1 {
		t.Fatalf("ordinary failure count = %d, want 1", got)
	}

	typed := NewKnowledge(nil)
	typed.Failed(gym, fmt.Errorf("wording can change completely: %w", gymOutcomeErr(gym, state.ResultLost)))
	if !combatLossRecorded(typed, gym) {
		t.Fatal("structured required gym loss was not classified as a gym loss")
	}

	structured := NewKnowledge(nil)
	structured.FailedResult(ObjectiveResult{
		Objective: gym,
		Battle:    &BattleEvidence{Result: "lost", Won: false},
	}, errors.New("native diagnostic wording is intentionally unrelated"))
	if !combatLossRecorded(structured, gym) {
		t.Fatal("structured BattleEvidence did not classify the gym loss")
	}
}

func TestTrainerLossClassificationRequiresTypedOrStructuredCause(t *testing.T) {
	trainer := Objective{Kind: KindTrainer, X: 10, Y: 6}

	proseOnly := NewKnowledge(nil)
	proseOnly.Failed(trainer, errors.New(skill.ErrTrainerBlackedOut.Error()))
	if combatLossRecorded(proseOnly, trainer) {
		t.Fatal("plain trainer-blackout prose was classified as a trainer loss")
	}

	typed := NewKnowledge(nil)
	typed.Failed(trainer, fmt.Errorf("renamed wrapper: %w", skill.ErrTrainerBlackedOut))
	if !combatLossRecorded(typed, trainer) {
		t.Fatal("typed trainer blackout was not classified as a trainer loss")
	}

	structured := NewKnowledge(nil)
	structured.FailedResult(ObjectiveResult{
		Objective: trainer,
		Cause:     FailureCauseID("trainer_blacked_out"),
	}, errors.New("native diagnostic wording is intentionally unrelated"))
	if !combatLossRecorded(structured, trainer) {
		t.Fatal("structured trainer_blacked_out cause did not classify the trainer loss")
	}
}

func TestGymOutcomeErrorCarriesStructuredLossSignal(t *testing.T) {
	gym := Objective{Kind: KindGym, Place: "pewter gym"}
	err := gymOutcomeErr(gym, state.ResultLost)
	if err == nil {
		t.Fatal("lost gym battle returned nil")
	}
	var required *skill.RequiredBattleError
	if !errors.As(err, &required) {
		t.Fatalf("gym loss error %v does not carry RequiredBattleError", err)
	}
	if required.Outcome.Encounter != "gym:pewter gym" ||
		required.Outcome.Result != state.ResultLost ||
		!required.Outcome.Trainer {
		t.Fatalf("gym battle outcome = %+v", required.Outcome)
	}
}
