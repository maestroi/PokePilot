package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

func TestSaffronGymRegistration(t *testing.T) {
	g, ok := GymAt(saffronGymMap)
	if !ok {
		t.Fatal("Saffron Gym is not registered as a gym challenge")
	}
	if g.Leader != "SABRINA" || g.Badge != state.BadgeMarsh {
		t.Fatalf("Saffron Gym metadata = leader %q badge %v, want SABRINA/Marsh", g.Leader, g.Badge)
	}
	if g.LeaderX != 9 || g.LeaderY != 8 {
		t.Fatalf("Sabrina tile = (%d,%d), want (9,8)", g.LeaderX, g.LeaderY)
	}

	dest, ok := Place("saffron gym")
	if !ok {
		t.Fatal("saffron gym place is not registered")
	}
	if dest != (Destination{Map: saffronGymMap, X: 9, Y: 9}) {
		t.Fatalf("saffron gym destination = %+v, want map %#02x at (9,9)", dest, saffronGymMap)
	}

	center, ok := Place("saffron pokemon center")
	if !ok || center.Map != saffronPokemonCenterMap || center.X != 3 || center.Y != 3 {
		t.Fatalf("saffron pokemon center = %+v, %v; want map %#02x at (3,3)", center, ok, saffronPokemonCenterMap)
	}
}

func TestIntraMapWarpDestinationUsesSelectedSourceWarp(t *testing.T) {
	warps := make([]rom.Warp, 4)
	warps[0] = rom.Warp{X: 1, Y: 3, DestWarpID: 3, DestMap: saffronGymMap}
	warps[1] = rom.Warp{X: 5, Y: 3, DestWarpID: 2, DestMap: saffronGymMap}
	warps[2] = rom.Warp{X: 11, Y: 9, DestWarpID: 1, DestMap: saffronGymMap}
	warps[3] = rom.Warp{X: 9, Y: 3, DestWarpID: 0, DestMap: saffronGymMap}
	h := rom.MapHeader{Warps: warps}

	e := world.Edge{Kind: world.EdgeWarp, From: saffronGymMap, To: saffronGymMap, WarpX: 1, WarpY: 3}
	x, y, err := intraMapWarpDestination(h, e)
	if err != nil {
		t.Fatalf("intraMapWarpDestination: %v", err)
	}
	if x != 9 || y != 3 {
		t.Fatalf("destination = (%d,%d), want selected source's warp index 3 at (9,3)", x, y)
	}
}

func TestIntraMapWarpDestinationRejectsWrongDestinationMap(t *testing.T) {
	h := rom.MapHeader{Warps: []rom.Warp{
		{X: 1, Y: 3, DestWarpID: 1, DestMap: 0x0A},
		{X: 9, Y: 3, DestWarpID: 0, DestMap: saffronGymMap},
	}}
	e := world.Edge{Kind: world.EdgeWarp, From: saffronGymMap, To: saffronGymMap, WarpX: 1, WarpY: 3}
	if _, _, err := intraMapWarpDestination(h, e); err == nil {
		t.Fatal("wrong-map source warp was accepted as an intra-map warp")
	}
}
