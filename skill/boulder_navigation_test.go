package skill

import (
	"errors"
	"testing"
)

// A nil policy is the plain-GoTo contract: the boulder solver must bubble
// interruptions to the navigation caller instead of fighting from inside
// pathing. The solver relies on RunInterruptible for that passthrough.
func TestBoulderNavigationWithoutPolicyBubblesInterruptions(t *testing.T) {
	for _, want := range []error{ErrBattleInterrupted, ErrDialogueInterrupted} {
		calls := 0
		_, err := RunInterruptible(nil, nil, InterruptibleAction{
			Name:           "boulder puzzle",
			MaxEngagements: boulderPuzzleMaxEngagements,
			Run:            func() error { calls++; return want },
		})
		if !errors.Is(err, want) || calls != 1 {
			t.Fatalf("nil policy: err=%v calls=%d, want %v after exactly one attempt", err, calls, want)
		}
	}
}
