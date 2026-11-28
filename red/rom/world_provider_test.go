package rom

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func TestGen1SurfWaterMatchesCollisionCheckOnWater(t *testing.T) {
	for _, tc := range []struct {
		tile, tileset uint8
		want          bool
	}{
		{0x14, 0x00, true},
		{0x32, 0x00, true},  // eastern coastline
		{0x32, 0x0e, false}, // SHIP_PORT: S.S. Anne gangway
		{0x48, 0x03, true},  // Safari coast
		{0x3a, 0x00, false},
	} {
		spec := worldmodel.GridSpec{Walkable: []bool{false}, CollisionTile: []uint8{tc.tile}, FieldTile: []uint8{0}}
		enableGen1SurfWater(&spec, tc.tileset)
		if spec.Walkable[0] != tc.want {
			t.Errorf("tile %#x tileset %#x surfable=%v, want %v", tc.tile, tc.tileset, spec.Walkable[0], tc.want)
		}
	}
}

// Route 20's west Seafoam door (48,5) is reached by Surf from the Fuchsia
// sea across $32 coastline tiles; the Cinnabar sea never reaches it.
// run-d6dokr184ky81 (#2778) saw the door's shore as a sealed water pocket.
func TestRoute20WestSeafoamShoreJoinsFuchsiaSea(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const route20 = 0x1f
	h, err := ParseMap(data, route20)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := Blocks(data, h)
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewWorldProvider(data).Grid(route20, blocks, worldmodel.TraversalWater)
	if err != nil {
		t.Fatal(err)
	}
	reach := func(x, y int) map[int]bool {
		seen := map[int]bool{y*g.Width + x: true}
		for q := []int{y*g.Width + x}; len(q) > 0; q = q[1:] {
			cx, cy := q[0]%g.Width, q[0]/g.Width
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := cx+d[0], cy+d[1]
				i := ny*g.Width + nx
				if nx >= 0 && ny >= 0 && nx < g.Width && ny < g.Height && g.Walkable[i] && !seen[i] {
					seen[i] = true
					q = append(q, i)
				}
			}
		}
		return seen
	}
	westDoorShore := 2*g.Width + 44
	if !reach(99, 4)[westDoorShore] {
		t.Fatal("Fuchsia-side sea (99,4) cannot Surf to the west Seafoam door shore (44,2)")
	}
	if reach(1, 4)[westDoorShore] {
		t.Fatal("Cinnabar-side sea (1,4) reaches the west Seafoam door shore; Route 20 should be split")
	}
}
