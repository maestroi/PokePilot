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
		{"Giovanni stand", giovanniStand},
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
	g, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(B4F): %v", err)
	}

	if _, err := world.FindPath(g, int(rocketB4FEntry.X), int(rocketB4FEntry.Y), int(giovanniStand.X), int(giovanniStand.Y), nil); err == nil {
		t.Fatal("static B4F grid reaches Giovanni through the closed boss door; live-door regression is no longer pinned")
	}
	applyLiveOpenBlock(g, 5, 12)
	if _, err := world.FindPath(g, int(rocketB4FEntry.X), int(rocketB4FEntry.Y), int(giovanniStand.X), int(giovanniStand.Y), nil); err != nil {
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
