package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

func badgeFourROM(t *testing.T) []byte {
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

func TestBadgeFourRoute10ToLavenderUsesRockTunnel(t *testing.T) {
	romData := badgeFourROM(t)
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	// Route 10's canonical fly point (11,20) is on the north component next
	// to the north Rock Tunnel entrance. A map-id-only planner can incorrectly
	// offer the south Lavender connection from here; the component-aware graph
	// must route through the cave and re-enter Route 10 on its south component.
	route, err := world.FindRouteAt(g, 0x15, 0x04, 11, 20, nil)
	if err != nil {
		t.Fatalf("FindRouteAt(Route10 north -> Lavender): %v", err)
	}
	if len(route) < 3 {
		t.Fatalf("route has %d edge(s): %v; want a Rock Tunnel detour, not the direct south connection", len(route), route)
	}
	if route[len(route)-1].To != 0x04 {
		t.Fatalf("last edge = %+v, want Lavender map 0x04", route[len(route)-1])
	}
	usedRockTunnel := false
	for _, e := range route {
		if e.From == 0x52 || e.To == 0x52 || e.From == 0xE8 || e.To == 0xE8 {
			usedRockTunnel = true
			break
		}
	}
	if !usedRockTunnel {
		t.Fatalf("Route10 north -> Lavender route does not use Rock Tunnel: %v", route)
	}
}

func TestBadgeFourMapsContainCutFieldTiles(t *testing.T) {
	romData := badgeFourROM(t)
	for _, tc := range []struct {
		name string
		mapID uint8
	}{
		{"route 9", 0x14},
		{"celadon city", celadonCityMap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := rom.ParseMap(romData, tc.mapID)
			if err != nil {
				t.Fatalf("ParseMap(%02x): %v", tc.mapID, err)
			}
			g, err := world.Build(romData, h)
			if err != nil {
				t.Fatalf("Build(%02x): %v", tc.mapID, err)
			}
			count := 0
			for y := 0; y < g.Height; y++ {
				for x := 0; x < g.Width; x++ {
					tile, ok := g.FieldTile(x, y)
					if ok && cutRouteTile(h.Tileset, tile) {
						count++
					}
				}
			}
			if count == 0 {
				t.Fatalf("map %02x has no field-action Cut tiles; Cut-aware routing cannot open its gate", tc.mapID)
			}
			t.Logf("map %02x exposes %d field-action Cut tile(s)", tc.mapID, count)
		})
	}
}
