package skill

import (
	"errors"
	"fmt"
	"testing"
)

func TestExhaustedConnectionBandIsEdgeScopedOnly(t *testing.T) {
	if !errors.Is(ErrConnectionBandExhausted, ErrLegUnwalkable) {
		t.Fatal("ErrConnectionBandExhausted must remain compatible with ErrLegUnwalkable")
	}

	err := fmt.Errorf("traverse failed after trying the whole band: %w", ErrConnectionBandExhausted)
	edge, tile := legFailureBanScope(err)
	if !edge || tile {
		t.Fatalf("exhausted-band scope = (edge=%v tile=%v), want edge-scoped only (true, false)", edge, tile)
	}
}

func TestOrdinaryConnectionFailureRemainsTileScoped(t *testing.T) {
	err := fmt.Errorf("one approach failed: %w", ErrLegUnwalkable)
	edge, tile := legFailureBanScope(err)
	if edge || !tile {
		t.Fatalf("ordinary unwalkable scope = (edge=%v tile=%v), want tile-scoped only (false, true)", edge, tile)
	}
}


func TestExhaustedConnectionBandIsFiniteWithoutReplanBudget(t *testing.T) {
	// The finite bound is the number of unique edge-scoped bands recorded in
	// deadEnds, not GoTo's small transient replan budget. Preserve the stronger
	// sentinel through wrapping so the caller can distinguish it from one-tile
	// ErrLegUnwalkable evidence.
	err := fmt.Errorf("all candidates failed: %w", ErrConnectionBandExhausted)
	if !errors.Is(err, ErrConnectionBandExhausted) {
		t.Fatal("wrapped exhausted-band evidence was lost")
	}
	if !errors.Is(err, ErrLegUnwalkable) {
		t.Fatal("exhausted-band evidence must remain navigation-compatible")
	}
}
