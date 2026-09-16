package skill

import (
	"errors"
	"fmt"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
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

// TestRouteFailureIsSpuriousCapabilityGate pins the fix for
// run-t047j1rjrshy3ob28cwqwr7pn: a *world.RouteBlockedError reported right
// after this same GoTo call already banned a leg for ErrLegUnwalkable must be
// treated as replan exhaustion, not as real prerequisite evidence — a
// RouteBlockedError with no leg banned this call, or any error with a leg
// banned, must NOT be reclassified.
func TestRouteFailureIsSpuriousCapabilityGate(t *testing.T) {
	blocked := &world.RouteBlockedError{Blockages: []gameruntime.TransitionBlockage{{
		Transition: gameruntime.Transition{ID: "red:cycling_road_bicycle"},
		Missing:    []gameruntime.CapabilityID{"can_ride_cycling_road"},
	}}}

	if !routeFailureIsSpuriousCapabilityGate(blocked, true) {
		t.Fatalf("RouteBlockedError after a same-call leg ban must be treated as spurious")
	}
	if routeFailureIsSpuriousCapabilityGate(blocked, false) {
		t.Fatalf("RouteBlockedError with no leg ever banned this call is real prerequisite evidence, not spurious")
	}
	if routeFailureIsSpuriousCapabilityGate(world.ErrNoRoute, true) {
		t.Fatalf("a plain no-route error is not a capability gate and must not be reclassified")
	}
}
