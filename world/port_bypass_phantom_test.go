package world

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// TestPortBypassRejectsPhantomConnectionBands locks the shared invariant
// behind farm run-2ccw7p3rpnvu4129l1dkhc7ayh: PortBypass may skip the
// standing-component check on a real exit port, but a connection band with
// no walkable exit tile is not a bridge. Taking it would land with a nil
// component set and let the next hop invent far-side exits.
func TestPortBypassRejectsPhantomConnectionBands(t *testing.T) {
	real := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast, BandStart: 1, BandEnd: 1, BandScoped: true}
	phantom := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast, BandStart: 0, BandEnd: 0, BandScoped: true}
	farExit := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirSouth, BandStart: 0, BandEnd: 0, BandScoped: true}
	bridge := Edge{Kind: EdgeWarp, From: 2, To: 3, WarpX: 0, WarpY: 0}

	g := &Graph{
		componentAware: true,
		Edges: map[uint8][]Edge{
			1: {phantom, real},
			2: {farExit, bridge},
			3: {},
		},
		comps: map[uint8][][]int{
			1: {{1, 1}},
			// Map 2 has two disconnected components: landing from `real` is
			// component 1; the south farExit lives only in component 2.
			2: {{1, 0}, {0, 2}},
			3: {{1}},
		},
		tiles: map[uint8]dim{1: {w: 2, h: 1}, 2: {w: 2, h: 2}, 3: {w: 1, h: 1}},
		exitComps: map[Edge][]int{
			real:    {1},
			farExit: {2},
			bridge:  {1},
		},
		entryComps: map[Edge][]int{
			real:    {1},
			farExit: {1},
			bridge:  {1},
		},
		// phantom deliberately omitted from exitComps/entryComps: len==0.
	}

	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_cut"),
		Transitions: map[Edge]gameruntime.Transition{
			real: {
				ID:         "cut_bridge",
				Requires:   []gameruntime.CapabilityID{"can_cut"},
				PivotOnly:  true,
				PortBypass: true,
			},
			phantom: {
				ID:         "cut_bridge",
				Requires:   []gameruntime.CapabilityID{"can_cut"},
				PivotOnly:  true,
				PortBypass: true,
			},
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 3, 0, 0, 0, 0, nil, prereqs)
	if err != nil {
		t.Fatalf("PortBypass on the real band should still route via the bridge: %v", err)
	}
	if len(plan) != 2 || plan[0].Edge != real || plan[1].Edge != bridge {
		t.Fatalf("plan=%+v, want real band then bridge (not phantom+farExit)", plan)
	}
	for _, step := range plan {
		if step.Edge == phantom || step.Edge == farExit {
			t.Fatalf("phantom PortBypass invented far-side exit: %+v", plan)
		}
	}
}
