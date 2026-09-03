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

	wild := battleBlackoutError(battleResolution{outcome: state.ResultLost})
	if !errors.Is(wild, ErrBlackedOut) {
		t.Fatalf("wild loss = %v, want ErrBlackedOut", wild)
	}
	if errors.Is(wild, ErrTrainerBlackedOut) {
		t.Fatalf("wild loss = %v, must not classify as trainer blackout", wild)
	}
}
