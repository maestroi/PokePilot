package skill

import (
	"errors"
	"testing"
)

func TestWaitForPickupFaceInterruptionCatchesDelayedBattle(t *testing.T) {
	frames := 0
	err := waitForPickupFaceInterruption(
		func() { frames++ },
		func() error {
			if frames >= 120 {
				return ErrBattleInterrupted
			}
			return nil
		},
	)
	if !errors.Is(err, ErrBattleInterrupted) {
		t.Fatalf("delayed interruption = %v, want ErrBattleInterrupted", err)
	}
	if frames != 120 {
		t.Fatalf("interruption detected after %d frames, want 120", frames)
	}
}

func TestWaitForPickupFaceInterruptionReturnsNilAfterIdleSettle(t *testing.T) {
	frames := 0
	err := waitForPickupFaceInterruption(
		func() { frames++ },
		func() error { return nil },
	)
	if err != nil {
		t.Fatalf("idle settle = %v, want nil", err)
	}
	if frames != pickupFaceInterruptionSettleFrames {
		t.Fatalf("waited %d frames, want bounded budget %d", frames, pickupFaceInterruptionSettleFrames)
	}
}
