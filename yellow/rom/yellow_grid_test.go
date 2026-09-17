package rom

import (
	"os"
	"testing"

	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// Pallet Town is the same map in both games and shares the Overworld tileset
// geometry, so Red's grid is the oracle for Yellow's. This pins both halves
// of the tileset decode:
//
//   - the BLOCK data (16 tile ids per block, resolved through the tileset
//     table's block pointer), and
//   - the COLLISION list (the walkable-tile ids, resolved through the entry's
//     collision pointer).
//
// Both moved between the games, and the collision pointer's bank is a
// build-layout fact the pointer cannot reveal (Yellow assembled the lists
// into bank 1, Red into bank 0; the game dereferences the pointer with no
// bank switch). A regression in either address or in CollisionListBank
// shows up here as a tile or walkability divergence.
func TestPalletGridMatchesRed(t *testing.T) {
	yellowData, err := os.ReadFile("../../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	redData, err := os.ReadFile("../../roms/pokemon_red.gb")
	if err != nil {
		t.Skip("pokemon_red.gb not available")
	}
	yh, err := ParseMap(yellowData, 0x00)
	if err != nil {
		t.Fatalf("yellow Pallet Town: %v", err)
	}
	rh, err := redrom.ParseMap(redData, 0x00)
	if err != nil {
		t.Fatalf("red Pallet Town: %v", err)
	}
	yg, err := world.BuildForTables(Tables(), yellowData, yh)
	if err != nil {
		t.Fatalf("yellow grid: %v", err)
	}
	rg, err := world.BuildForTables(redrom.RedTables(), redData, rh)
	if err != nil {
		t.Fatalf("red grid: %v", err)
	}
	if yg.Width != rg.Width || yg.Height != rg.Height {
		t.Fatalf("grid sizes: yellow %dx%d, red %dx%d", yg.Width, yg.Height, rg.Width, rg.Height)
	}
	tileDiff, walkDiff := 0, 0
	for y := 0; y < yg.Height; y++ {
		for x := 0; x < yg.Width; x++ {
			yt, _ := yg.Tile(x, y)
			rt, _ := rg.Tile(x, y)
			if yt != rt {
				tileDiff++
			}
			if yg.Walkable(x, y) != rg.Walkable(x, y) {
				walkDiff++
			}
		}
	}
	if tileDiff != 0 {
		t.Errorf("tile ids differ at %d of %d tiles", tileDiff, yg.Width*yg.Height)
	}
	if walkDiff != 0 {
		t.Errorf("walkability differs at %d of %d tiles", walkDiff, yg.Width*yg.Height)
	}
}
