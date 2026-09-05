package skill

import (
	"errors"
	"testing"
)

func TestErrCatchBlackoutIsRecoverableBlackout(t *testing.T) {
	if !errors.Is(ErrCatchBlackout, ErrCatchBlackout) {
		t.Fatal("ErrCatchBlackout no longer matches its specific sentinel")
	}
	if !errors.Is(ErrCatchBlackout, ErrBlackedOut) {
		t.Fatalf("ErrCatchBlackout = %v, want it to unwrap to ErrBlackedOut", ErrCatchBlackout)
	}
	const want = "skill: Catch: blacked out while fighting a non-wanted battle"
	if ErrCatchBlackout.Error() != want {
		t.Fatalf("ErrCatchBlackout text = %q, want %q", ErrCatchBlackout, want)
	}
}
