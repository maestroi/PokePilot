package skill

import (
	"errors"
	"testing"
)

func TestBoulderNavigationWithoutPolicyBubblesBattle(t *testing.T) {
	err := resolveBoulderWalkInterruption(nil, nil, ErrBattleInterrupted)
	if !errors.Is(err, ErrBattleInterrupted) {
		t.Fatalf("resolveBoulderWalkInterruption(nil policy) = %v, want ErrBattleInterrupted", err)
	}
}

func TestBoulderNavigationWithoutPolicyBubblesDialogue(t *testing.T) {
	err := resolveBoulderWalkInterruption(nil, nil, ErrDialogueInterrupted)
	if !errors.Is(err, ErrDialogueInterrupted) {
		t.Fatalf("resolveBoulderWalkInterruption(nil policy) = %v, want ErrDialogueInterrupted", err)
	}
}
