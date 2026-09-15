package skill

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

// TestSafeForcedBanIncludesPreviouslyCommittedDeadEnds reproduces the shape
// behind farm issues #729/#730. A candidate revisit ban can look safe when the
// trial route is allowed to use an edge that GoTo already classified as a
// map-scoped dead end earlier in the same journey. Committing that second ban
// then leaves only a capability-gated route in the live planner.
func TestSafeForcedBanIncludesPreviouslyCommittedDeadEnds(t *testing.T) {
	const (
		current = uint8(0x01)
		visited = uint8(0x02)
		detour  = uint8(0x03)
		gated   = uint8(0x04)
		destMap = uint8(0x05)
	)

	forcedEdge := world.Edge{Kind: world.EdgeConnection, From: current, To: visited}
	detourEdge := world.Edge{Kind: world.EdgeConnection, From: current, To: detour}
	staleDeadEnd := world.Edge{Kind: world.EdgeConnection, From: detour, To: destMap}
	gatedEdge := world.Edge{Kind: world.EdgeConnection, From: current, To: gated}
	gatedToDest := world.Edge{Kind: world.EdgeConnection, From: gated, To: destMap}
	visitedToDest := world.Edge{Kind: world.EdgeConnection, From: visited, To: destMap}

	g := &world.Graph{Edges: map[uint8][]world.Edge{
		current: {forcedEdge, detourEdge, gatedEdge},
		visited: {visitedToDest},
		detour:  {staleDeadEnd},
		gated:   {gatedToDest},
		destMap: nil,
	}}

	prereqs := world.RoutePrerequisites{
		Transitions: map[world.Edge]gameruntime.Transition{
			gatedEdge: {
				ID:       "red:route12_snorlax_access",
				From:     "lavender town",
				To:       "route 12",
				Requires: []gameruntime.CapabilityID{"can_clear_snorlax"},
				Gate:     true,
			},
		},
		Capabilities: gameruntime.NewCapabilitySet(),
	}

	forced := legFromMap{e: forcedEdge, m: current}
	deadEnds := map[legFromMap]bool{
		legFromMap{e: staleDeadEnd, m: detour}: true,
	}
	dest := Destination{Map: destMap}

	// Without applying staleDeadEnd, the detour makes the candidate ban look
	// safe. That is precisely the false-positive the live GoTo accumulated.
	if _, _, safe := safeForcedBan(g, current, dest, 0, 0, nil, forced, prereqs); !safe {
		t.Fatal("test premise changed: legacy isolated-ban trial no longer sees the stale detour")
	}

	_, err, safe := safeForcedBanWithDeadEnds(g, current, dest, 0, 0, nil, forced, deadEnds, prereqs)
	if safe {
		t.Fatal("cumulative ban trial approved a route that depends on an already-banned downstream edge")
	}
	var blocked *world.RouteBlockedError
	if err == nil || !errors.As(err, &blocked) {
		t.Fatalf("cumulative trial error = %v, want RouteBlockedError", err)
	}
	missing := blocked.MissingCapabilities()
	if len(missing) != 1 || missing[0] != "can_clear_snorlax" {
		t.Fatalf("missing capabilities = %v, want [can_clear_snorlax]", missing)
	}
}
