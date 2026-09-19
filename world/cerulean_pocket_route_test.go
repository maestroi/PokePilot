package world

import (
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// TestCeruleanBadgeHousePocketDoesNotPlanUnreachableRoute9Exit is the
// regression behind farm triage aaf93af50f5e9d50 (run-27p5q00agjl0p180cgtm32v2d1)
// and triage 9c8108f027ccf462 (run-ow5n52dpiovo3dhxp31zd5jzx).
//
// With can_cut, Route 9's PivotOnly annotation used to skip FROM-side canExit,
// so a player trapped in Cerulean's isolated Badge House north pocket at (9,9)
// planned "east to Route 9" — a connection band no tile in that pocket can
// reach. GoTo then either exhausted its re-plan budget or fell into the Badge
// House and bounced 03(9,9)->e6(2,1)->03(9,9) until navigation_stalled.
// PivotOnly must keep FROM-side reachability; the escape is Badge House in,
// plaza south door out — never the north door back into the same pocket.
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
	if len(plan) < 2 {
		t.Fatalf("pocket -> Celadon returned %d leg(s), want at least Badge House in + plaza out", len(plan))
	}
	first := plan[0].Edge
	if first.Kind == EdgeConnection && first.To == route9 {
		t.Fatalf("pocket planned unreachable east Route 9 connection: %+v", first)
	}
	if first.Kind != EdgeWarp || first.To != badge || first.WarpX != 9 || first.WarpY != 9 {
		t.Fatalf("first hop = %+v, want Badge House north-pocket door (9,9)", first)
	}
	second := plan[1].Edge
	if second.Kind != EdgeWarp || second.From != badge || second.To != cerulean || second.WarpY != 7 {
		t.Fatalf("second hop = %+v, want Badge House south plaza exit (WarpY=7), not the north pocket door", second)
	}
}

// TestCeruleanEastDoesNotBounceThroughRoute9ToReachRoute4West is farm #1261:
// standing on Cerulean's east seam with can_cut, bidirectional Route 9
// PivotOnly planned Cerulean -> Route 9 -> Cerulean -> Route 4. That re-entry
// relaxes Cerulean's landing and unlocks plaza-only Route 4 bands the east
// seam cannot walk, so GoTo oscillates until navigation_stalled.
func TestCeruleanEastDoesNotBounceThroughRoute9ToReachRoute4West(t *testing.T) {
	if os.Getenv("POKEMON_RED_ROM") == "" {
		t.Skip("POKEMON_RED_ROM required")
	}
	g := loadGraph(t)
	transitions := map[Edge]gameruntime.Transition{}
	for _, edges := range [][]Edge{g.Edges[0x03], g.Edges[0x14]} {
		for _, e := range edges {
			if e.Kind != EdgeConnection {
				continue
			}
			if (e.From == 0x03 && e.To == 0x14) || (e.From == 0x14 && e.To == 0x03) {
				transitions[e] = gameruntime.Transition{
					ID:        "red:route9_cut",
					Requires:  []gameruntime.CapabilityID{"can_cut"},
					PivotOnly: true,
				}
			}
		}
	}
	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_cut"),
		Transitions:  transitions,
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(
		g, 0x03, 0x0f, 39, 17, 10, 10, nil, prereqs,
	)
	if err != nil {
		return
	}
	if len(plan) >= 2 && plan[0].Edge.To == 0x14 && plan[1].Edge.To == 0x03 {
		t.Fatalf("Cerulean east planned Route 9 bounce toward Route 4 west: %+v", plan)
	}
}
