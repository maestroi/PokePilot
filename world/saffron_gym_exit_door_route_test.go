package world

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

// TestSaffronGymExitDoorRoutesViaReachableTeleporter is the ROM-backed pin for
// the go_to saffron gym route_replan_exhausted burst measured on:
//
//   - run-opn83x1wj99vp9241oyhcacb (#1414)
//   - run-3ryd6j6etrlvo3eof6m6vuiu6l / triage:978e898d718fbf45 / farm-issue #1412
//   - run-1uyafua3jt0o9egm3y6lg9np3 / triage:6ffbb6bf79d245b0 / farm-issue #1413
//
// Standing on the gym exit warp (8,17), FindRouteAtDestination toward Place
// (9,9) must take the entrance-room teleporter (11,15) first — never the
// unreachable far pad (5,9) that pre-#1414 offered when standingComponentAt
// returned nil on the door tile and canExit stopped filtering.
//
// World Explorer: https://rompilot.app/world?map=SAFFRON_GYM&x=8&y=17&debug=1
func TestSaffronGymExitDoorRoutesViaReachableTeleporter(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	const saffronGym uint8 = 0xb2
	h, err := rom.ParseMap(romData, saffronGym)
	if err != nil {
		t.Fatalf("ParseMap: %v", err)
	}
	grid, err := Build(romData, h)
	if err != nil {
		t.Fatalf("Build grid: %v", err)
	}
	g2, err := g.WithMapGrid(saffronGym, grid)
	if err != nil {
		t.Fatalf("WithMapGrid: %v", err)
	}

	const (
		doorX, doorY       = 8, 17
		placeX, placeY     = 9, 9
		nearPadX, nearPadY = 11, 15
		farPadX, farPadY   = 5, 9
	)

	route, err := FindRouteAtDestination(g2, saffronGym, saffronGym, doorX, doorY, placeX, placeY, nil)
	if err != nil {
		t.Fatalf("FindRouteAtDestination from exit door (8,17) to Place (9,9): %v", err)
	}
	if len(route) == 0 {
		t.Fatal("expected at least one same-map warp hop from the exit door; got empty route")
	}
	first := route[0]
	if first.Kind != EdgeWarp || first.From != saffronGym || first.To != saffronGym {
		t.Fatalf("first leg = %+v, want same-map warp on SAFFRON_GYM", first)
	}
	if int(first.WarpX) == farPadX && int(first.WarpY) == farPadY {
		t.Fatalf("first leg chose unreachable far pad (5,9); want entrance teleporter (%d,%d)", nearPadX, nearPadY)
	}
	if int(first.WarpX) != nearPadX || int(first.WarpY) != nearPadY {
		t.Fatalf("first leg = warp (%d,%d), want reachable entrance teleporter (%d,%d)",
			first.WarpX, first.WarpY, nearPadX, nearPadY)
	}
}
