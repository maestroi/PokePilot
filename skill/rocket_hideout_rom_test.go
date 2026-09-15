package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

func rocketHideoutROM(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read POKEMON_RED_ROM: %v", err)
	}
	return b
}

func TestRocketHideoutTargetsAreWalkable(t *testing.T) {
	romData := rocketHideoutROM(t)
	for _, d := range []struct {
		name string
		dest Destination
	}{
		{"game corner stand", gameCornerStand},
		{"B3F return", rocketB3FReturn},
		{"B4F entry", rocketB4FEntry},
	} {
		t.Run(d.name, func(t *testing.T) {
			h, err := rom.ParseMap(romData, d.dest.Map)
			if err != nil {
				t.Fatalf("ParseMap(%02x): %v", d.dest.Map, err)
			}
			g, err := world.Build(romData, h)
			if err != nil {
				t.Fatalf("Build(%02x): %v", d.dest.Map, err)
			}
			if !g.Walkable(int(d.dest.X), int(d.dest.Y)) {
				t.Fatalf("%s = map %02x (%d,%d) is not walkable", d.name, d.dest.Map, d.dest.X, d.dest.Y)
			}
		})
	}
}

func TestGameCornerSecretDoorNeedsLiveOverride(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, gameCornerMap)
	if err != nil {
		t.Fatalf("ParseMap(Game Corner): %v", err)
	}
	g, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(Game Corner): %v", err)
	}

	if _, _, err := world.FindPathAdjacent(g, int(gameCornerStand.X), int(gameCornerStand.Y), int(gameCornerWarpX), int(gameCornerWarpY), nil); err == nil {
		t.Fatal("static Game Corner grid reaches secret stair before poster override; live-door regression is no longer pinned")
	}
	applyLiveOpenBlock(g, 2, 8)
	if _, _, err := world.FindPathAdjacent(g, int(gameCornerStand.X), int(gameCornerStand.Y), int(gameCornerWarpX), int(gameCornerWarpY), nil); err != nil {
		t.Fatalf("secret stair still unreachable after live block override: %v", err)
	}
}

func TestRocketBossDoorNeedsLiveOverride(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		t.Fatalf("ParseMap(B4F): %v", err)
	}
	// RocketHideoutB4FDoorCallbackScript writes block $2d (door block) at
	// bc=(5,12) while the two guards are unbeaten. This ROM's compiled B4F
	// map already ships that cell as $0e (MEASURED), so build the closed
	// state explicitly instead of assuming world.Build's raw bytes reflect
	// it.
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		t.Fatalf("Blocks(B4F): %v", err)
	}
	closedBlocks := append([]byte(nil), blocks...)
	closedBlocks[5*int(h.WidthBlocks)+12] = 0x2d
	g, err := world.BuildFromBlocks(romData, h, closedBlocks)
	if err != nil {
		t.Fatalf("BuildFromBlocks(B4F closed): %v", err)
	}

	// rocketB4FEntry (the west/stair side) is not walkably connected to the
	// guards' side at all, door or no door (RocketHideout's own comment on
	// reachRocketGuardSide); the only foot route to Giovanni starts from the
	// elevator's B4F landing tile (25,15), one of its own def_warp_events.
	// giovanniStand (25,4) is the desk tile in front of Giovanni, not itself
	// walkable (MEASURED: only (25,3), Giovanni's own tile, borders it), so
	// route to a tile adjacent to it, the same as walkRocketBossDoor does.
	const elevatorLandingX, elevatorLandingY = 25, 15
	if _, _, err := world.FindPathAdjacent(g, elevatorLandingX, elevatorLandingY, int(giovanniStand.X), int(giovanniStand.Y), nil); err == nil {
		t.Fatal("closed-door B4F grid reaches Giovanni through the boss door; live-door regression is no longer pinned")
	}
	applyLiveOpenBlock(g, 5, 12)
	if _, _, err := world.FindPathAdjacent(g, elevatorLandingX, elevatorLandingY, int(giovanniStand.X), int(giovanniStand.Y), nil); err != nil {
		t.Fatalf("Giovanni still unreachable after live boss-door override: %v", err)
	}
}

func TestRocketHideoutB1FCanRouteToB4F(t *testing.T) {
	romData := rocketHideoutROM(t)
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	route, err := world.FindRouteAt(g, rocketHideoutB1FMap, rocketHideoutB4FMap, 21, 3, nil)
	if err != nil {
		t.Fatalf("FindRouteAt(B1F -> B4F): %v", err)
	}
	if len(route) == 0 || route[len(route)-1].To != rocketHideoutB4FMap {
		t.Fatalf("B1F -> B4F route = %v", route)
	}
}
