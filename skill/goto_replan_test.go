package skill

import (
	"errors"
	"fmt"
	"testing"
)

// TestReplanExhaustedKeepsBothIdentities pins the S5c-2 contract: the
// exhaustion error must be recognisable as terminal (ErrReplanExhausted)
// AND still carry the cause of the last failed leg (ErrLegUnwalkable), so
// a policy checking errors.Is(err, ErrLegUnwalkable) cannot mistake a
// terminal give-up for a recoverable single-leg failure. It exercises the
// constructor directly rather than driving nine real route attempts
// against the ROM, so it stays pure and fast.
func TestReplanExhaustedKeepsBothIdentities(t *testing.T) {
	last := fmt.Errorf("route 2 north edge: %w", ErrLegUnwalkable)
	err := newReplanExhaustedError(8, 0x0d, 8, 71,
		Destination{Map: 0x02, X: 14, Y: 8}, last)
	if !errors.Is(err, ErrReplanExhausted) {
		t.Fatalf("missing ErrReplanExhausted: %v", err)
	}
	if !errors.Is(err, ErrLegUnwalkable) {
		t.Fatalf("missing ErrLegUnwalkable cause: %v", err)
	}
}
