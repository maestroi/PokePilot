package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestTrainerBlackoutIsNarrowSubtypeOfBlackout(t *testing.T) {
	if !errors.Is(ErrTrainerBlackedOut, ErrBlackedOut) {
		t.Fatal("ErrTrainerBlackedOut must remain an ErrBlackedOut for existing recovery callers")
	}
	if errors.Is(ErrBlackedOut, ErrTrainerBlackedOut) {
		t.Fatal("ordinary blackout must not classify as a trainer blackout")
	}
}

func TestBattleBlackoutErrorDistinguishesTrainerFromWild(t *testing.T) {
	trainer := battleBlackoutError(battleResolution{outcome: state.ResultLost, trainer: true})
	if !errors.Is(trainer, ErrTrainerBlackedOut) || !errors.Is(trainer, ErrBlackedOut) {
		t.Fatalf("trainer loss = %v, want both trainer and broad blackout classes", trainer)
	}
	var required *RequiredBattleError
	if !errors.As(trainer, &required) || required.Outcome.Result != state.ResultLost || !required.Outcome.Trainer {
		t.Fatalf("trainer loss = %v, want structured required trainer battle outcome", trainer)
	}

	wild := battleBlackoutError(battleResolution{outcome: state.ResultLost})
	if !errors.Is(wild, ErrBlackedOut) {
		t.Fatalf("wild loss = %v, want ErrBlackedOut", wild)
	}
	if errors.Is(wild, ErrTrainerBlackedOut) {
		t.Fatalf("wild loss = %v, must not classify as trainer blackout", wild)
	}
}


func TestRecordTravelBattleDefeatPreservesSemanticKind(t *testing.T) {
	trainerResult := TravelResult{}
	trainerErr := recordTravelBattleDefeat(&trainerResult, battleResolution{outcome: state.ResultLost, trainer: true})
	if !trainerResult.BlackedOut || !trainerResult.TrainerDefeat {
		t.Fatalf("trainer result = %+v, want blackout with trainer defeat evidence", trainerResult)
	}
	if !errors.Is(trainerErr, ErrTrainerBlackedOut) {
		t.Fatalf("trainer err = %v, want legacy trainer blackout compatibility", trainerErr)
	}
	var required *RequiredBattleError
	if !errors.As(trainerErr, &required) || required.Outcome.Result != state.ResultLost || !required.Outcome.Trainer {
		t.Fatalf("trainer err = %v, want structured required battle evidence", trainerErr)
	}

	wildResult := TravelResult{}
	wildErr := recordTravelBattleDefeat(&wildResult, battleResolution{outcome: state.ResultLost})
	if !wildResult.BlackedOut || wildResult.TrainerDefeat {
		t.Fatalf("wild result = %+v, want ordinary blackout without trainer defeat evidence", wildResult)
	}
	if !errors.Is(wildErr, ErrBlackedOut) || errors.Is(wildErr, ErrTrainerBlackedOut) {
		t.Fatalf("wild err = %v, want only broad blackout compatibility", wildErr)
	}
}
