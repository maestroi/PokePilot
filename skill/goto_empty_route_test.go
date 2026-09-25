package skill

import (
	"errors"
	"strings"
	"testing"
)

// Farm #1488 (triage 3231d9e75777f5a9) ended a run while travelling from
// GAME_CORNER (0x87) to VERMILION_CITY (0x05): an impossible empty cross-map
// route fell through to walkWithinMap, whose map mismatch was untyped and
// therefore normalized to terminal unknown_failure/unknown_error.
func TestEmptyCrossMapRouteIsTypedNavigationFailure(t *testing.T) {
	dest := MapDestination(0x05)
	err := emptyCrossMapRouteError(0x87, 0, 0, dest)
	if err == nil {
		t.Fatal("empty cross-map route returned nil")
	}
	if !errors.Is(err, ErrNavigationStalled) {
		t.Fatalf("empty cross-map route error = %v, want ErrNavigationStalled", err)
	}
	if !strings.Contains(err.Error(), "87") || !strings.Contains(err.Error(), "05") {
		t.Fatalf("empty cross-map route error lacks map evidence: %v", err)
	}
}

func TestEmptyRouteOnDestinationMapMayFinalizeLocally(t *testing.T) {
	if err := emptyCrossMapRouteError(0x05, 11, 4, MapDestination(0x05)); err != nil {
		t.Fatalf("same-map empty route = %v, want nil", err)
	}
}
