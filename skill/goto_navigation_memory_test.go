package skill

import (
	"errors"
	"testing"
)

// TestNavigationMemorySurvivesBattleReentry pins the fix for
// run-13kws9zfzq7ka1p4bmd16c9yg3: Travel's retry loop calls GoTo again after
// every battle it resolves, and each of those calls used to build its own
// fresh navigationGuard/visitedMaps/deadEnds — discarding everything the
// journey had already learned about which legs bounce. A wild battle right at
// the Rock Tunnel 1F / Route 10 boundary reset visitedMaps every retry, so the
// replanned route detoured back in through Route 9 and re-triggered a battle
// at the same tile, burning the whole maxBattles budget with zero net
// progress.
//
// A single shared *navigationMemory across those calls is the fix; this pins
// the exact invariant that makes it work: the SAME guard survives a later
// call, and it still refuses a repeated (map, tile), even when that repeat is
// only visible because two calls are stitched together, not one.
func TestNavigationMemorySurvivesBattleReentry(t *testing.T) {
	dest := Destination{Map: 0x02, X: 14, Y: 8}
	nav := newNavigationMemory()

	// First GoTo call for this journey: starts at the Rock Tunnel 1F
	// boundary tile, takes one real step onto Route 10.
	rockTunnel := navigationState{Map: 0x52, X: 15, Y: 7}
	route10 := navigationState{Map: 0x15, X: 8, Y: 17}
	first := nav.ensureGuard(dest, rockTunnel)
	if err := first.observe(route10); err != nil {
		t.Fatalf("first leg: %v", err)
	}

	// A wild battle interrupts here; Travel resolves it and calls GoTo again.
	// The player is back at essentially the same tile the battle started at
	// (battles never move the player), so this is a resumed call, not a new
	// journey.
	second := nav.ensureGuard(dest, rockTunnel)
	if second != first {
		t.Fatalf("ensureGuard replaced the journey's guard on a resumed call; every bounce fact already learned is lost")
	}

	// The bug's exact shape: the replanned route from the resumed position
	// detours back through the same (map, tile) this journey already
	// crossed. That must be caught as a stall, not silently re-walked.
	if err := second.observe(route10); !errors.Is(err, ErrNavigationStalled) {
		t.Fatalf("observe(%+v) = %v, want ErrNavigationStalled: a battle-boundary bounce across two GoTo calls must be caught", route10, err)
	}
}
