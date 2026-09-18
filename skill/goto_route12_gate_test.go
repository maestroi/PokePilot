package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// TestVisitedMapPreferenceAllowsRoute12GateComponentBridge is the graph-level
// regression for #1035. Reaching Fuchsia from Route 12's north side requires
// entering Route 12 Gate 1F (0x57) and then returning to Route 12 (0x17) in a
// DIFFERENT walkable component. The old anti-bounce preference remembered only
// the map byte, so all Gate -> Route 12 exits looked like revisits even though
// the south doors are real forward progress.
func TestVisitedMapPreferenceAllowsRoute12GateComponentBridge(t *testing.T) {
	p := os.Getenv("POKEMON_RED_ROM")
	if p == "" {
		t.Skip("POKEMON_RED_ROM required")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	g, err := world.BuildGraph(data)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	const (
		route12 = uint8(0x17)
		gate1F  = uint8(0x57)
	)
	h, err := rom.ParseMap(data, route12)
	if err != nil {
		t.Fatalf("ParseMap Route 12: %v", err)
	}
	grid, err := world.Build(data, h)
	if err != nil {
		t.Fatalf("Build Route 12 grid: %v", err)
	}
	warpTiles := map[[2]int]bool{}
	for _, w := range h.Warps {
		warpTiles[[2]int{int(w.X), int(w.Y)}] = true
	}

	// Pick a real standing tile beside the north Route 12 -> Gate entrance,
	// rather than baking a component id into the test.
	var visited navigationState
	foundVisit := false
	for _, e := range g.Edges[route12] {
		if e.To != gate1F || e.Kind != world.EdgeWarp || e.WarpY != 15 {
			continue
		}
		for _, d := range [][2]int{{0, -1}, {-1, 0}, {1, 0}, {0, 1}} {
			x, y := int(e.WarpX)+d[0], int(e.WarpY)+d[1]
			if !grid.InBounds(x, y) || !grid.Walkable(x, y) || warpTiles[[2]int{x, y}] {
				continue
			}
			visited = navigationState{Map: route12, X: uint8(x), Y: uint8(y)}
			foundVisit = true
			break
		}
		if foundVisit {
			break
		}
	}
	if !foundVisit {
		t.Fatal("could not find a walkable Route 12 tile beside the north Gate entrance")
	}

	visitedMaps := map[uint8]bool{route12: true}
	visitedPositions := map[uint8][]navigationState{route12: {visited}}
	filtered := graphWithoutEdgesInto(g, visitedMaps, visitedPositions)
	kept := map[world.Edge]bool{}
	for _, e := range filtered.Edges[gate1F] {
		kept[e] = true
	}

	same, fresh := 0, 0
	for _, e := range g.Edges[gate1F] {
		if e.To != route12 || e.Kind != world.EdgeWarp {
			continue
		}
		revisit, known := g.EdgeEntrySharesComponentWith(e, int(visited.X), int(visited.Y))
		if !known {
			t.Fatalf("missing component evidence for Gate edge %+v from Route 12 visit %+v", e, visited)
		}
		if revisit {
			same++
			if kept[e] {
				t.Fatalf("same-component Gate return remained eligible: %+v", e)
			}
			continue
		}
		fresh++
		if !kept[e] {
			t.Fatalf("fresh-component Gate return was incorrectly banned: %+v", e)
		}
		if forced, ok := forcedRevisitBan(
			g, []world.RouteStep{{Edge: e}}, nil, visitedMaps, map[legFromMap]bool{}, visitedPositions,
		); ok {
			t.Fatalf("fresh-component Gate return was misclassified as a forced bounce: %+v", forced)
		}
	}
	if same == 0 || fresh == 0 {
		t.Fatalf("Route 12 Gate premise changed: same-component exits=%d fresh-component exits=%d", same, fresh)
	}
}
