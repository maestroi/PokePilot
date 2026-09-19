package world

import (
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// TestCeruleanBadgeHousePocketDoesNotPlanUnreachableRoute9Exit is the
// regression behind farm triage aaf93af50f5e9d50 (run-27p5q00agjl0p180cgtm32v2d1).
// With can_cut, Route 9's PivotOnly annotation used to skip FROM-side canExit,
// so a player trapped in Cerulean's isolated Badge House north pocket at (9,9)
// planned "east to Route 9" — a connection band no tile in that pocket can
// reach — and GoTo exhausted its re-plan budget. PivotOnly must keep FROM-side
// reachability; the first hop from the pocket has to be the Badge House warp.
func TestCeruleanBadgeHousePocketDoesNotPlanUnreachableRoute9Exit(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM required")
	}
	data, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	g, err := BuildGraph(data)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	const (
		cerulean = uint8(0x03)
		route9   = uint8(0x14)
		celadon  = uint8(0x06)
		badge    = uint8(0xe6)
	)
	var route9Edge Edge
	found := false
	for _, e := range g.Edges[cerulean] {
		if e.Kind == EdgeConnection && e.To == route9 {
			route9Edge = e
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Cerulean -> Route 9 connection missing from graph")
	}
	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_cut"),
		Transitions: map[Edge]gameruntime.Transition{
			route9Edge: {
				ID:        "red:route9_cut",
				Requires:  []gameruntime.CapabilityID{"can_cut"},
				PivotOnly: true,
			},
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(
		g, cerulean, celadon, 9, 9, 41, 10, nil, prereqs,
	)
	if err != nil {
		t.Fatalf("pocket -> Celadon: %v", err)
	}
	if len(plan) == 0 {
		t.Fatal("pocket -> Celadon returned empty route")
	}
	first := plan[0].Edge
	if first.Kind == EdgeConnection && first.To == route9 {
		t.Fatalf("pocket planned unreachable east Route 9 connection: %+v", first)
	}
	if first.Kind != EdgeWarp || first.To != badge {
		t.Fatalf("first hop = %+v, want Badge House warp from the (9,9) pocket", first)
	}
}
