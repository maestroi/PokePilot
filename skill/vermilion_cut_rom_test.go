package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// TestVermilionGymWarpHasDestinationAwareCutRoute pins the real-ROM shape that
// used to be handled by a nearest-tree scan. Starting from the ordinary
// Vermilion City place, at least one approach tile for the *selected Gym warp*
// must be reachable by the shared field planner, and that exact route must
// contain Cut. This proves the tree is chosen because it opens the requested
// door, not merely because it is nearby and reachable.
func TestVermilionGymWarpHasDestinationAwareCutRoute(t *testing.T) {
	romData := badgeFourROM(t)
	start, ok := Place("vermilion city")
	if !ok {
		t.Fatal("vermilion city place missing")
	}
	h, err := rom.ParseMap(romData, vermilionCity)
	if err != nil {
		t.Fatalf("ParseMap(vermilion): %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(vermilion): %v", err)
	}
	graph, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	var gymEdge *world.Edge
	for _, e := range graph.Edges[vermilionCity] {
		if e.Kind == world.EdgeWarp && e.To == vermilionGymMap {
			copy := e
			gymEdge = &copy
			break
		}
	}
	if gymEdge == nil {
		t.Fatalf("no Vermilion City -> Gym warp edge")
	}

	var best []fieldPathStep
	for _, w := range edgeWarpCandidates(h, *gymEdge, romData) {
		for _, side := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
			x, y := int(w.X)+side.DX, int(w.Y)+side.DY
			if !grid.InBounds(x, y) || !grid.Walkable(x, y) {
				continue
			}
			plan, perr := planFieldPath(
				grid, nil, h.Tileset,
				int(start.X), int(start.Y), x, y,
				nil, true, false, false,
			)
			if perr != nil {
				continue
			}
			if best == nil || len(plan) < len(best) {
				best = plan
			}
		}
	}
	if best == nil {
		t.Fatalf("no destination-aware Cut route from Vermilion place (%d,%d) to Gym warp", start.X, start.Y)
	}
	if got := countFieldActions(best, fieldPathCut); got != 1 {
		t.Fatalf("Gym warp plan Cut actions=%d, want exactly 1; plan=%+v", got, best)
	}
}
