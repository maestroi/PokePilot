package skill

import (
	"errors"
	"testing"
)

// TestErrLegBouncesBackDoesNotDegradeToTileBan locks the shared invariant
// behind farm run-3gf4z15byn2we2cyhg1zfez04n (and run-5r5c0f2hrowk387f36ca57zob):
// a measured bounce-back is evidence about the CONNECTION, not the tile it was
// crossed from. ErrLegBouncesBack wraps ErrLegUnwalkable so generic callers keep
// working, but GoTo must treat the stronger sentinel as map-scoped only —
// falling through to a per-tile ban rediscovers the same forced descent from
// another tile of the same map until navigation_stalled fires.
func TestErrLegBouncesBackDoesNotDegradeToTileBan(t *testing.T) {
	if !errors.Is(ErrLegBouncesBack, ErrLegUnwalkable) {
		t.Fatal("ErrLegBouncesBack must wrap ErrLegUnwalkable so existing Is checks keep working")
	}

	bounce, tile := legFailureBanScope(ErrLegBouncesBack)
	if !bounce || tile {
		t.Fatalf("bounce scope = (bounce=%v tile=%v), want map-scoped only (true, false)", bounce, tile)
	}

	plain := errors.New("approach blocked")
	wrappedPlain := errors.Join(plain, ErrLegUnwalkable)
	bounce, tile = legFailureBanScope(wrappedPlain)
	if bounce || !tile {
		t.Fatalf("plain unwalkable scope = (bounce=%v tile=%v), want tile-scoped only (false, true)", bounce, tile)
	}

	other := errors.New("unrelated traverse failure")
	bounce, tile = legFailureBanScope(other)
	if bounce || tile {
		t.Fatalf("unrelated failure scope = (bounce=%v tile=%v), want no ban (false, false)", bounce, tile)
	}
}
