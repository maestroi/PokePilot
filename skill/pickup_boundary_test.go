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

func TestPickupFaceExhaustionIsRecoverableNavigationStall(t *testing.T) {
	err := pickupFaceExhaustedError(6, 8, errors.New("face timed out"))
	if !errors.Is(err, ErrNavigationStalled) {
		t.Fatalf("exhaustion = %v, want ErrNavigationStalled", err)
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
