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
	err := fmt.Errorf("all candidates failed: %w", ErrConnectionBandExhausted)
	if legFailureConsumesReplanBudget(err) {
		t.Fatal("fully exhausted band must not consume transient GoTo replan budget")
	}
	if !errors.Is(err, ErrLegUnwalkable) {
		t.Fatal("exhausted-band evidence must remain navigation-compatible")
	}

	for _, other := range []error{ErrLegUnwalkable, ErrLegBouncesBack, errors.New("other")} {
		if !legFailureConsumesReplanBudget(other) {
			t.Fatalf("%v unexpectedly bypassed replan budget", other)
		}
	}
}
