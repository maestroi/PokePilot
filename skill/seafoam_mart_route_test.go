package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestRoute20WestMartSelectionDoesNotLureThroughSeafoam locks
// run-d6dokr184ky81 / triage ac2c16d5fd9372ab: from Route 20's Cinnabar-side
// water, indigo plateau must not win nearest-mart selection via a one-hop
// red:seafoam_islands replan frontier while Cinnabar's shop is a completed
// two-hop route. The shared world prune for requirement-free puzzle-cave
// warps is what removes that lure; this pins the mart-facing symptom.
func TestRoute20WestMartSelectionDoesNotLureThroughSeafoam(t *testing.T) {
	romData := badgeFourROM(t)

	mem := fieldTestMem(FieldSurf, true, true, true)
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade |
		1<<state.BadgeThunder | 1<<state.BadgeRainbow | 1<<state.BadgeSoul |
		1<<state.BadgeMarsh | 1<<state.BadgeVolcano | 1<<state.BadgeEarth
	setRouteTestEvent(mem, state.EventGotPokedex)
	cut, ok := FieldMoveSpecFor(FieldCut)
	if !ok {
		t.Fatal("Cut field move missing")
	}
	mem[sym.PartyMon1+sym.MonMoves+1] = cut.MoveID
	setTestBag(mem, [2]uint8{hm03SurfItem, 1}, [2]uint8{cut.HMItem, 1}, [2]uint8{0x49, 1})
	// Route 20 west shore beside Cinnabar — the farm checkpoint shape.
	mem[sym.CurMap] = route20Map
	mem[sym.XCoord] = 0
	mem[sym.YCoord] = 4

	g, err := routeGraphForROM(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	g, err = withSurfSeaTopology(g, romData, mem)
	if err != nil {
		t.Fatalf("withSurfSeaTopology: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, mem)

	indigo := standardMartRecoveryTarget{name: "indigo plateau mart", mapID: 0xAE}
	cinnabar := standardMartRecoveryTarget{name: "cinnabar mart", mapID: 0xAC}
	indigoRoute, indigoErr := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route20Map, indigo.mapID, 0, 4, standardMartCounterX, standardMartCounterY, nil, prereqs,
	)
	if errors.Is(indigoErr, world.ErrRouteReplanRequired) {
		for i, step := range indigoRoute {
			if step.Transition != nil && step.Transition.ID == "red:seafoam_islands" {
				t.Fatalf("indigo still lures through Seafoam at leg %d: err=%v plan=%+v", i, indigoErr, indigoRoute)
			}
		}
	}
	cinnabarRoute, cinnabarErr := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route20Map, cinnabar.mapID, 0, 4, standardMartCounterX, standardMartCounterY, nil, prereqs,
	)
	if cinnabarErr != nil && !errors.Is(cinnabarErr, world.ErrRouteReplanRequired) {
		t.Fatalf("cinnabar route: %v", cinnabarErr)
	}
	if len(cinnabarRoute) == 0 {
		t.Fatal("cinnabar route empty")
	}

	mart, found, err := nearestMartWhere(romData, mem, func(standardMartRecoveryTarget) bool { return true })
	if err != nil || !found {
		t.Fatalf("nearestMartWhere: ok=%v err=%v", found, err)
	}
	if mart.mapID != cinnabar.mapID {
		t.Fatalf("nearest mart = %s (%#02x), want cinnabar (%#02x); indigoRoute=%+v/%v cinnabarRoute=%+v/%v",
			mart.name, mart.mapID, cinnabar.mapID, indigoRoute, indigoErr, cinnabarRoute, cinnabarErr)
	}
}

func TestSeafoamIslandsTransitionExecutorIsOwned(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeWarp, From: route20Map, To: seafoam1FMap}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok || transition.ID != "red:seafoam_islands" {
		t.Fatalf("transition = %+v ok=%v, want red:seafoam_islands", transition, ok)
	}
	result, handled, err := (&redRouteTransitionExecutor{}).executeAuditedRouteTransition(edge, transition)
	if !handled || err != nil {
		t.Fatalf("seafoam executor handled=%v err=%v", handled, err)
	}
	if result.Changed {
		t.Fatalf("seafoam pivot should be a no-op, got %+v", result)
	}
}
