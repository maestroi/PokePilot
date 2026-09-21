package skill

import (
	"errors"
	"testing"
)

func TestPickupInterruptionForTravelPromotesBattleToTravelOwnership(t *testing.T) {
	err := pickupInterruptionForTravel(ErrBattleInterrupted)
	if !errors.Is(err, ErrBattle) {
		t.Fatalf("battle interruption = %v, want ErrBattle", err)
	}
}

func TestPickupInterruptionForTravelPreservesDialogue(t *testing.T) {
	err := pickupInterruptionForTravel(ErrDialogueInterrupted)
	if !errors.Is(err, ErrDialogueInterrupted) {
		t.Fatalf("dialogue interruption = %v, want ErrDialogueInterrupted", err)
	}
}

func TestPickupInterruptionForTravelPreservesNil(t *testing.T) {
	if err := pickupInterruptionForTravel(nil); err != nil {
		t.Fatalf("nil interruption = %v, want nil", err)
	}
}
