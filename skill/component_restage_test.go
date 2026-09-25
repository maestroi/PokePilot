package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

// TestTileOpensDestRejectsUnwalkable pins the Route 16 gate wall pitfall from
// run-2cs2fsg74sw2h31bfqwes80gq5: FindRoutePlan reports a route from an
// unwalkable neighbour of a warp landing because component filtering treats
// component-0 non-warp tiles as unknown. Restaging must refuse those tiles.
func TestTileOpensDestRejectsUnwalkable(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatal(err)
	}
	center, ok := Place("celadon pokemon center")
	if !ok {
		t.Fatal(`Place("celadon pokemon center") missing`)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	prereqs := redRoutePrerequisites(g, romData, &mem)
	prereqs.Capabilities = gameruntime.NewCapabilitySet(capCanCut)

	if tileOpensDest(m, romData, g, route16Gate1FMap, 0, 1, center, prereqs, nil) {
		t.Fatal("tileOpensDest must reject unwalkable gate tile (0,1)")
	}
	if !tileOpensDest(m, romData, g, route16Map, 24, 6, center, prereqs, nil) {
		t.Fatal("tileOpensDest must accept Route 16 east-north (24,6) via Cut bridge")
	}
}
