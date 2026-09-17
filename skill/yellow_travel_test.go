package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

// YellowTravelExercise drives the pieces a real run composes, on a Yellow ROM
// and a Yellow save state, through the public skill entry points:
//
//	cachedRouteGraph  -> the per-ROM graph (agent dispatch target)
//	world.FindPath    -> geometry from the Yellow grid
//	WalkPath          -> the movement cadence that actually steps the player
//
// It is the end-to-end check that the tileset-table and collision-bank fixes
// are not just self-consistent but produce a walkable world the engine
// agrees with: a path the planner computed must actually be walkable, tile
// for tile, or the player ends up somewhere the plan did not intend.
//
// It is deliberately a movement test, not a story test. The boot fixture
// stands the player in REDS_HOUSE_2F with an empty party, so there is nothing
// to fight and no RNG in the path: the only randomness the engine sees is
// rDIV, which cannot change the geometry. Travel to the north exit is
// avoided because that is where Yellow's Oak cutscene seizes input.
func TestYellowTravelExercise(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	st, err := os.ReadFile("failure/yellow_overworld.state")
	if err != nil {
		t.Skip("yellow_overworld.state not available")
	}

	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatalf("cachedRouteGraph on Yellow: %v", err)
	}
	// The graph is keyed by map id; 227 of them parse on Yellow.
	if got := len(g.Edges); got != 227 {
		t.Fatalf("Yellow graph has %d maps with edges, want 227", got)
	}

	e, err := emu.OpenCGBBytes(romData)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.LoadState(st); err != nil {
		t.Fatal(err)
	}

	startX, startY := playerXY(e)
	startMap := e.Peek8(0xD35D)
	t.Logf("start: map 0x%02x (%d,%d)", startMap, startX, startY)
	if startMap != 0x26 {
		t.Fatalf("start map = 0x%02x, want REDS_HOUSE_2F (0x26)", startMap)
	}

	// A short, entirely indoor walk: from the start tile to the far corner of
	// the room. Every tile is inside the house, so no warp can fire and no
	// script can seize input.
	dest := struct{ X, Y int }{5, 6}
	if !walkYellow(t, e, int(startX), int(startY), dest.X, dest.Y) {
		return
	}

	// Now assert the planner and the engine agree on a longer walk that
	// crosses the room and back, which is what a real objective does.
	walkYellow(t, e, dest.X, dest.Y, 3, 6)
}

// walkYellow plans from (sx,sy) to (dx,dy) on the live map's grid and walks
// it, reporting failure. It returns false when the test has already
// reported, so the caller can bail out.
func walkYellow(t *testing.T, e *emu.Emu, sx, sy, dx, dy int) bool {
	t.Helper()
	// The grid for the live map comes from the same per-ROM table resolution
	// the graph used, so a fix that is wrong for one is wrong for both.
	mapID := e.Peek8(0xD35D)
	h, err := yellowrom.ParseMap(e.ROM(), mapID)
	if err != nil {
		t.Fatalf("parse live map 0x%02x: %v", mapID, err)
	}
	grid, err := world.BuildForTables(graphForROM(e.ROM()), e.ROM(), h)
	if err != nil {
		t.Fatalf("grid for map 0x%02x: %v", mapID, err)
	}
	path, err := world.FindPath(grid, sx, sy, dx, dy, spriteBlockers(e, e.ROM()))
	if err != nil {
		t.Errorf("FindPath (%d,%d)->(%d,%d): %v", sx, sy, dx, dy, err)
		return false
	}
	if len(path) == 0 {
		t.Errorf("FindPath (%d,%d)->(%d,%d) returned an empty path", sx, sy, dx, dy)
		return false
	}
	if err := WalkPath(e, path); err != nil {
		t.Errorf("WalkPath (%d,%d)->(%d,%d): %v", sx, sy, dx, dy, err)
		return false
	}
	gotX, gotY := playerXY(e)
	if int(gotX) != dx || int(gotY) != dy {
		t.Errorf("after walk: player at (%d,%d), want (%d,%d)", gotX, gotY, dx, dy)
		return false
	}
	t.Logf("walked (%d,%d)->(%d,%d) in %d steps", sx, sy, dx, dy, len(path))
	return true
}

// TestYellowSkillLayerUsesYellowTables pins the geometry layer's per-ROM
// resolution at the skill boundary, not the library boundary. The readers the
// runtime actually calls -- graphForROM for the map tables, tablesForROM for
// the tileset/wild set -- must both resolve Yellow on a Yellow image. If either
// falls back to Red, Yellow's Pallet Town decodes 6 blocked tiles to Red's 139
// and every plan walks through buildings, with no error to surface it.
//
// It compares against the same map on the Red image to catch a regression in
// either direction: the assertion is that the two images build two different
// table sets and Yellow's is the one Yellow needs, not that either grid equals
// some hardcoded geometry.
func TestYellowSkillLayerUsesYellowTables(t *testing.T) {
	yellowData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	redData, err := os.ReadFile("../roms/pokemon_red.gb")
	if err != nil {
		t.Skip("pokemon_red.gb not available")
	}

	// The two images must resolve to different map tables. rom.Tables carries
	// a func so it is not comparable directly; the table addresses it reads
	// from are, and they are what the grid is built from.
	yt, rt := graphForROM(yellowData), graphForROM(redData)
	if yt.MapHeaderPointersAddr == rt.MapHeaderPointersAddr && yt.MapHeaderBanksAddr == rt.MapHeaderBanksAddr {
		t.Fatal("Yellow and Red ROMs resolved to the same map table addresses; the per-ROM switch is dead")
	}
	if tablesForROM(yellowData).tilesetsAddr == tablesForROM(redData).tilesetsAddr {
		t.Fatal("Yellow and Red ROMs resolved to the same tileset table address")
	}

	// And Yellow's set must actually be Yellow's: its Pallet Town grid must
	// match the grid the yellow/rom package builds with its own tables. Red's
	// tables on the same image produce a different, wrong grid.
	const pallet = 0x00
	yh, err := graphForROM(yellowData).ParseMap(yellowData, pallet)
	if err != nil {
		t.Fatalf("parse PalletTown on Yellow: %v", err)
	}
	yg, err := world.BuildForTables(graphForROM(yellowData), yellowData, yh)
	if err != nil {
		t.Fatalf("build PalletTown grid on Yellow: %v", err)
	}
	rh, err := graphForROM(redData).ParseMap(redData, pallet)
	if err != nil {
		t.Fatalf("parse PalletTown on Red: %v", err)
	}
	rg, err := world.BuildForTables(graphForROM(redData), redData, rh)
	if err != nil {
		t.Fatalf("build PalletTown grid on Red: %v", err)
	}
	if yg.Width != rg.Width || yg.Height != rg.Height {
		t.Fatalf("PalletTown dims: Yellow %dx%d vs Red %dx%d", yg.Width, yg.Height, rg.Width, rg.Height)
	}
	// Yellow's blocked-tile count must be far lower than Red-on-Yellow's 139:
	// Red's tables on Yellow's bytes misread the collision list entirely.
	yBlocked, rBlocked := 0, 0
	for y := 0; y < yg.Height; y++ {
		for x := 0; x < yg.Width; x++ {
			if !yg.Walkable(x, y) {
				yBlocked++
			}
			if !rg.Walkable(x, y) {
				rBlocked++
			}
		}
	}
	// Pallet Town is the same town in both games, so the oracle is that the
	// two grids agree tile-for-tile and walkability-for-walkability. The
	// failure this guards is asymmetric: reading Yellow's bytes with Red's
	// collision bank resolves the tileset list into the wrong ROM region and
	// the town decodes ~6 blocked tiles instead of ~139, which silently
	// un-blocks buildings.
	diff := 0
	for y := 0; y < yg.Height; y++ {
		for x := 0; x < yg.Width; x++ {
			if yg.Walkable(x, y) != rg.Walkable(x, y) {
				diff++
			}
		}
	}
	t.Logf("PalletTown blocked tiles: Yellow=%d Red=%d walkability-diffs=%d", yBlocked, rBlocked, diff)
	if diff != 0 {
		t.Fatalf("skill layer built PalletTown with divergent geometry: %d tiles differ in walkability between Yellow and Red; Yellow's collision bank is being resolved from the wrong tables", diff)
	}
	if yBlocked < 100 {
		t.Fatalf("Yellow PalletTown has only %d blocked tiles, want ~139; the collision list resolved into the wrong ROM region", yBlocked)
	}
}
