package skill

import (
	"errors"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestSingleDestinationWarpExitRequiresOneExternalWarpNeighbor(t *testing.T) {
	g := &world.Graph{Edges: map[uint8][]world.Edge{
		1: {
			{Kind: world.EdgeWarp, From: 1, To: 2, WarpX: 2, WarpY: 7},
			{Kind: world.EdgeWarp, From: 1, To: 2, WarpX: 3, WarpY: 7},
		},
	}}
	edge, ok := singleDestinationWarpExit(g, 1)
	if !ok {
		t.Fatal("paired door warps to one external map were not recognized as a mandatory exit")
	}
	if edge.To != 2 {
		t.Fatalf("mandatory exit target = %#04x, want %#04x", edge.To, 2)
	}

	g.Edges[1] = append(g.Edges[1], world.Edge{Kind: world.EdgeWarp, From: 1, To: 3})
	if _, ok := singleDestinationWarpExit(g, 1); ok {
		t.Fatal("multi-neighbor room was incorrectly treated as a mandatory exit")
	}

	g.Edges[1] = []world.Edge{{Kind: world.EdgeConnection, From: 1, To: 2}}
	if _, ok := singleDestinationWarpExit(g, 1); ok {
		t.Fatal("connection-only map was incorrectly treated as a mandatory warp room")
	}
}

func TestRoute16FlyHouseCenterRecoveryUsesMandatoryExit(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	// This is the exact planner shape from the virtual-trade failure today:
	// the Fly House's LAST_MAP doors land in Route 16's Cut-gated pocket, so
	// static component routing may be unable to prove a complete route to any
	// Center while the player is still inside the house. A future graph that
	// can prove that route directly is also valid; any other error is not.
	if _, _, err := nearestPokemonCenterInGraph(g, route16FlyHouseMap); err != nil && !errors.Is(err, ErrPCNoKnownCenter) {
		t.Fatalf("Fly House static Center selection err = %v, want nil or ErrPCNoKnownCenter", err)
	}

	exit, ok := singleDestinationWarpExit(g, route16FlyHouseMap)
	if !ok {
		t.Fatal("Route 16 Fly House was not recognized as a single-destination warp room")
	}
	if exit.To != route16Map {
		t.Fatalf("Fly House mandatory exit goes to %#04x, want Route 16 %#04x", exit.To, route16Map)
	}

	// Re-planning after that measured hop is enough to select a Center. The
	// live TravelFlee call then owns the Route 16 Cut bridge from the actual
	// landing component.
	center, name, err := nearestPokemonCenterInGraph(g, route16Map)
	if err != nil {
		t.Fatalf("Center selection after mandatory Fly House exit: %v", err)
	}
	if name == "" || center.Map == 0 {
		t.Fatalf("Center selection after Fly House exit = %q %+v, want a concrete Center", name, center)
	}
}
