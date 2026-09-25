package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

func TestEdgeTargetForConnectionExcludingRetriesAnotherBorderTile(t *testing.T) {
	spec := worldmodel.GridSpec{
		MapID:         0x00,
		Width:         5,
		Height:        3,
		Walkable:      make([]bool, 15),
		CollisionTile: make([]uint8, 15),
		FieldTile:     make([]uint8, 15),
	}
	for i := range spec.Walkable {
		spec.Walkable[i] = true
	}
	grid, err := world.GridFromSpec(spec)
	if err != nil {
		t.Fatalf("GridFromSpec: %v", err)
	}
	edge := world.Edge{Kind: world.EdgeConnection, From: 0x00, To: 0x20, Dir: 1}

	x, y, err := edgeTargetForConnectionExcluding(grid, edge, 2, 1, nil, nil)
	if err != nil {
		t.Fatalf("first edge target: %v", err)
	}
	if x != 2 || y != 2 {
		t.Fatalf("first edge target = (%d,%d), want nearest south tile (2,2)", x, y)
	}

	excluded := map[[2]int]bool{{2, 2}: true}
	x, y, err = edgeTargetForConnectionExcluding(grid, edge, 2, 1, nil, excluded)
	if err != nil {
		t.Fatalf("retry edge target: %v", err)
	}
	if x != 1 || y != 2 {
		t.Fatalf("retry edge target = (%d,%d), want next deterministic south tile (1,2)", x, y)
	}
}
