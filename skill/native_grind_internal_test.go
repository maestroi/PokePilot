package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// The player stands on a door warp (2,2); the nearest grind pair must be two
// floor tiles that are neither warps nor occupied, reached without crossing
// the warp row.
func TestNearestGrindPairSkipsWarpsAndObjects(t *testing.T) {
	const w, h = 4, 3
	rows := []string{
		"#..#",
		"#..#",
		"#.W#",
	}
	spec := worldmodel.NativeGridSpec{Width: w, Height: h,
		Walkable: make([]bool, w*h), CollisionTile: make([]uint8, w*h), WarpTrigger: make([]bool, w*h)}
	for y, row := range rows {
		for x, c := range row {
			spec.Walkable[y*w+x] = c != '#'
			spec.WarpTrigger[y*w+x] = c == 'W'
		}
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	blocked := map[[2]int]bool{{2, 2}: true, {1, 1}: true}
	pair, ok := nearestGrindPair(grid, 2, 2, blocked)
	if !ok {
		t.Fatal("no grind pair found")
	}
	for _, c := range pair {
		if blocked[c] || !grid.Walkable(c[0], c[1]) || grid.WarpTriggers(c[0], c[1]) {
			t.Fatalf("pair %v includes a blocked/warp/wall tile", pair)
		}
	}
	if pair != [2][2]int{{2, 1}, {2, 0}} {
		t.Fatalf("pair = %v, want the floor just inside the door", pair)
	}
}
