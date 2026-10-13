package skill

import (
	"os"
	"slices"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

// TestRoute14SealedGrassReplanFrontier pins the invariant behind
// run-d6dokr184ky81 (triage 6f350c6398c961f4, issue 2601): Route 14's grass
// sits in a sealed pocket that no entry can reach, so a replan frontier that
// lands on an unrelated map (the planner detours through a Surf seam on
// another map) must not mark the habitat reachable.
//
// The test is deterministic and needs no emulator: it builds the graph from
// the ROM, overlays the Surf sea topology directly, and constructs the Surf
// prerequisites by hand.
func TestRoute14SealedGrassReplanFrontier(t *testing.T) {
	romData, err := os.ReadFile(os.Getenv("POKEMON_RED_ROM"))
	if err != nil {
		t.Skipf("no ROM: %v", err)
	}
	g, err := world.BuildGraphForROM(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	// Overlay the Surf sea topology the same way withSurfSeaTopology does,
	// but without a live state: every map that touches a Surf PortBypass
	// seam gets its water grid.
	provider, err := routingProviderForROM(romData)
	if err != nil {
		t.Fatalf("routingProviderForROM: %v", err)
	}
	seas := map[uint8]bool{}
	var surfEdges []world.Edge
	for _, edges := range g.Edges {
		for _, edge := range edges {
			if edge.Kind != world.EdgeConnection {
				continue
			}
			if tr, ok := redRouteTransitionForEdge(edge); ok && tr.PortBypass && slices.Contains(tr.Requires, capCanSurf) {
				seas[edge.From], seas[edge.To] = true, true
				surfEdges = append(surfEdges, edge)
			}
		}
	}
	if len(surfEdges) == 0 {
		t.Fatal("no Surf edges found in graph; ROM may be wrong")
	}
	for id := range seas {
		if _, overlaid := g.MapTraversal(id); overlaid {
			continue
		}
		spec, err := provider.Grid(id, nil, world.TraversalWater)
		if err != nil {
			t.Fatalf("provider.Grid(%#04x, water): %v", id, err)
		}
		grid, err := world.GridFromSpec(spec)
		if err != nil {
			t.Fatalf("GridFromSpec(%#04x): %v", id, err)
		}
		if g, err = g.WithMapGrid(id, grid); err != nil {
			t.Fatalf("WithMapGrid(%#04x): %v", id, err)
		}
	}

	// Construct the Surf prerequisites: the transitions on the Surf edges and
	// the player's Surf capability.
	transitions := make(map[world.Edge]gameruntime.Transition, len(surfEdges))
	for _, edge := range surfEdges {
		if tr, ok := redRouteTransitionForEdge(edge); ok {
			transitions[edge] = tr
		}
	}
	prereqs := world.RoutePrerequisites{
		Transitions:  transitions,
		Capabilities: gameruntime.NewCapabilitySet(capCanSurf),
	}

	// The player stands on Route 13 (0x18) at the top edge, the position the
	// failed run left them in. From here Route 14's grass is sealed: the
	// component-aware planner cannot reach the grass pocket from any entry.
	const (
		curMap = uint8(0x18)
		curX   = 51
		curY   = 0
	)

	// Route 14's grass must be reported unreachable: the replan frontier
	// lands on an unrelated map, not on Route 14 itself.
	if grassRoutable(g, prereqs, curMap, curX, curY, romData, 0x19) {
		t.Error("Route 14 grass reported reachable from Route 13 (51,0): a replan frontier on an unrelated map must not mark a sealed habitat reachable")
	}

	// Route 15's grass, by contrast, is reachable from the same position and
	// must stay reported reachable: the fix must not over-suppress.
	if !grassRoutable(g, prereqs, curMap, curX, curY, romData, 0x1a) {
		t.Error("Route 15 grass reported unreachable from Route 13 (51,0): the fix over-suppressed a reachable habitat")
	}
}
