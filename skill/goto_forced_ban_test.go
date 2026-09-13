package skill

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

// TestSafeForcedBanRefusesToStrandDestinationBehindMissingCapability
// reproduces run-2f9zwq0xjhcvp2wvui3kzvw12q: SSAnneHM01's GoTo, standing in
// Cerulean's trashed house, bounced into Cerulean City and back once (a
// legitimate component bridge, not a real dead end). forcedRevisitBan cannot
// tell that apart from the Route 4/Route 24 dead-end bounces it exists to
// catch, so it flags the trashed-house<->Cerulean edge for banning. Banning
// it stranded the only Cut-free path to Vermilion (through Route 5) and left
// only Route 9's Cut-gated pivot in the graph, so GoTo died on "missing
// capabilities [can_cut]" despite Cut never being required for this trip.
// safeForcedBan must refuse the ban here: the destination becomes reachable
// only through a capability the traveler does not have.
func TestSafeForcedBanRefusesToStrandDestinationBehindMissingCapability(t *testing.T) {
	const (
		trashedHouse = uint8(0x3e)
		cerulean     = uint8(0x03)
		route5       = uint8(0x10)
		route9       = uint8(0x14)
		vermilion    = uint8(0x05)
	)
	houseToCerulean := world.Edge{Kind: world.EdgeWarp, From: trashedHouse, To: cerulean, WarpX: 27, WarpY: 11}
	ceruleanToHouse := world.Edge{Kind: world.EdgeWarp, From: cerulean, To: trashedHouse, WarpX: 27, WarpY: 11}
	ceruleanToRoute5 := world.Edge{Kind: world.EdgeConnection, From: cerulean, To: route5}
	route5ToVermilion := world.Edge{Kind: world.EdgeConnection, From: route5, To: vermilion}
	ceruleanToRoute9 := world.Edge{Kind: world.EdgeConnection, From: cerulean, To: route9}
	// Not real Kanto geography (Route 9 does not actually reach Vermilion) —
	// this stand-in edge only exists so the "cut available" half of the test
	// below can exercise a real, non-stranding alternate route through the
	// Cut pivot without modeling all of Rock Tunnel/Lavender/Route 6.
	route9ToVermilion := world.Edge{Kind: world.EdgeConnection, From: route9, To: vermilion}

	g := &world.Graph{Edges: map[uint8][]world.Edge{
		trashedHouse: {houseToCerulean},
		cerulean:     {ceruleanToHouse, ceruleanToRoute5, ceruleanToRoute9},
		route5:       {route5ToVermilion},
		route9:       {route9ToVermilion},
		vermilion:    nil,
	}}

	route9Cut := gameruntime.Transition{
		ID:       "red:route9_cut",
		From:     "cerulean city",
		To:       "route 9",
		Requires: []gameruntime.CapabilityID{"can_cut"},
	}
	prereqs := world.RoutePrerequisites{
		Transitions:  map[world.Edge]gameruntime.Transition{ceruleanToRoute9: route9Cut},
		Capabilities: gameruntime.NewCapabilitySet(), // no can_cut
	}
	dest := Destination{Map: vermilion, X: 0, Y: 0}

	// The bounce this call is meant to model: with Route 5 already visited
	// once (an earlier leg of the same GoTo call), the only forward move
	// once the "don't revisit" preference is dropped re-enters it —
	// geometrically identical to the Route 4/Route 24 dead-end bounces
	// forcedRevisitBan exists to catch.
	retry := []world.RouteStep{{Edge: ceruleanToRoute5}}
	visitedMaps := map[uint8]bool{route5: true}
	deadEnds := map[legFromMap]bool{}

	forced, ok := forcedRevisitBan(g, retry, nil, visitedMaps, deadEnds)
	if !ok {
		t.Fatalf("forcedRevisitBan did not flag the bounce edge; premise of the regression is gone")
	}
	if forced.e != ceruleanToRoute5 || forced.m != cerulean {
		t.Fatalf("forced ban = %+v, want the Cerulean->Route 5 edge", forced)
	}

	_, _, safe := safeForcedBan(g, cerulean, dest, 0, 0, nil, forced, prereqs)
	if safe {
		t.Fatalf("safeForcedBan approved a ban that strands Vermilion behind can_cut")
	}

	// Once Cut is available, the same ban is fine: Route 9 becomes a real
	// alternative, so refusing it would be overly conservative.
	withCut := prereqs
	withCut.Capabilities = gameruntime.NewCapabilitySet()
	withCut.Capabilities["can_cut"] = true
	if _, _, safe := safeForcedBan(g, cerulean, dest, 0, 0, nil, forced, withCut); !safe {
		t.Fatalf("safeForcedBan refused a ban that no longer strands the destination once can_cut is held")
	}
}
