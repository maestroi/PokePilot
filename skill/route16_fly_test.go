package skill

import (
	"errors"
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestRoute16FlyHouseTransactionDestination(t *testing.T) {
	got, ok := Place(route16FlyHousePlace)
	if !ok {
		t.Fatal("Route 16 Fly house transaction destination is not registered")
	}
	want := Destination{Map: route16FlyHouseMap, X: route16FlyHouseStagingX, Y: route16FlyHouseStagingY}
	if got != want {
		t.Fatalf("Route 16 Fly house destination = %+v, want %+v", got, want)
	}
	approach, ok := Place(route16FlyApproachPlace)
	if !ok {
		t.Fatal("Route 16 Fly approach destination is not registered")
	}
	wantApproach := Destination{Map: route16Map, X: route16FlyApproachX, Y: route16FlyApproachY}
	if approach != wantApproach {
		t.Fatalf("Route 16 Fly approach = %+v, want %+v", approach, wantApproach)
	}
}

// TestRoute16FlyHouseRejectsLowerGateTeleport pins farm triage
// 0ece8bd130597547 / run-1g7kwah5oygzl29jkvhmhsmby6: with flute+bike+cut,
// semantic routing used to treat the lower Cycling Road gate door (24,10) as
// a component teleport onto the upper pedestrian exits and plan
// Route16 -lower-gate-> Gate1F -upper-west-> Fly house. The player entered the
// lower corridor and died with world: no route from (7,8). The lower-door
// annotation must stay a Gate so landing remains the lower corridor; the
// honest Fly-house path uses the upper gate warps at y=4/5.
func TestRoute16FlyHouseRejectsLowerGateTeleport(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	var mem state.Mem
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade | 1<<state.BadgeThunder |
		1<<state.BadgeRainbow | 1<<state.BadgeSoul | 1<<state.BadgeMarsh
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 44 // Oddish
	copy(mem.Slice(base+sym.MonMoves, 4), []byte{cutMove, 0, 0, 0})
	mem[sym.NumBagItems] = 3
	mem[sym.BagItems] = hm01Item
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = bicycleItem
	mem[sym.BagItems+3] = 1
	mem[sym.BagItems+4] = pokeFluteItem
	mem[sym.BagItems+5] = 1
	mem[sym.BagItems+6] = 0xff
	setEventFlag(&mem, eventBeatRoute16Snorlax)

	prereqs := redRoutePrerequisites(g, romData, &mem)
	for _, need := range []gameruntime.CapabilityID{capCanCut, capCanRideCyclingRoad, capCanClearSnorlax} {
		if !prereqs.Capabilities.Has(need) {
			t.Fatalf("capabilities missing %q: %v", need, prereqs.Capabilities)
		}
	}

	dest, ok := Place(route16FlyHousePlace)
	if !ok {
		t.Fatal("Fly house place missing")
	}

	assertNoLowerGate := func(t *testing.T, label string, from uint8, x, y int) []world.RouteStep {
		t.Helper()
		plan, err := world.FindRoutePlanAtDestinationWithCapabilities(
			g, from, dest.Map, x, y, int(dest.X), int(dest.Y), nil, prereqs,
		)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		for i, step := range plan {
			e := step.Edge
			if e.From == route16Map && e.To == route16Gate1FMap &&
				e.WarpX == 24 && (e.WarpY == 10 || e.WarpY == 11) {
				t.Fatalf("%s leg %d used lower gate warp (%d,%d); plan=%+v", label, i+1, e.WarpX, e.WarpY, plan)
			}
			if step.Transition != nil && step.Transition.ID == "red:route16_snorlax_bicycle" {
				t.Fatalf("%s leg %d still carries lower-gate transition: %+v", label, i+1, step)
			}
		}
		return plan
	}

	plan := assertNoLowerGate(t, "upper east (24,5)", route16Map, 24, 5)
	if len(plan) < 2 || plan[0].Edge.WarpY != 4 && plan[0].Edge.WarpY != 5 {
		t.Fatalf("upper east plan = %+v, want first hop through upper gate y=4/5", plan)
	}

	_, err = world.FindRoutePlanAtDestinationWithCapabilities(
		g, route16Gate1FMap, dest.Map, 7, 8, int(dest.X), int(dest.Y), nil, prereqs,
	)
	if !errors.Is(err, world.ErrNoRoute) {
		t.Fatalf("lower gate (7,8)->fly: err=%v, want ErrNoRoute", err)
	}

	_, err = world.FindRoutePlanAtDestinationWithCapabilities(
		g, route16Map, dest.Map, 24, 10, int(dest.X), int(dest.Y), nil, prereqs,
	)
	if !errors.Is(err, world.ErrNoRoute) {
		t.Fatalf("lower east (24,10)->fly land plan: err=%v, want ErrNoRoute (Cut bridge owns the seam)", err)
	}
}
