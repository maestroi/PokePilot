package world

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// TestSurfPortBypassRoutesAcrossWaterOnlySeams locks the shared invariant
// behind farm triage ff3ad54bb21d3a2d (run-3uwfb4to132uw2i3ohvjn1kuge):
// Surf PortBypass edges (skipCanExit+relaxLanding) cross water seams whose
// land exitComps are empty by construction. The phantom-band filter from
// #1338 must keep rejecting PivotOnly+PortBypass Cut padding, but must not
// erase every southern-sea / Route 21 Surf shore.
func TestSurfPortBypassRoutesAcrossWaterOnlySeams(t *testing.T) {
	shore := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirSouth, BandStart: 0, BandEnd: 3, BandScoped: true}
	arrival := Edge{Kind: EdgeWarp, From: 2, To: 3, WarpX: 1, WarpY: 1}

	g := &Graph{
		componentAware: true,
		Edges: map[uint8][]Edge{
			1: {shore},
			2: {arrival},
			3: {},
		},
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1}},
			3: {{1}},
		},
		tiles: map[uint8]dim{1: {w: 1, h: 1}, 2: {w: 2, h: 2}, 3: {w: 2, h: 2}},
		// Water shore: no land component on either seam tile.
		exitComps:  map[Edge][]int{arrival: {1}},
		entryComps: map[Edge][]int{arrival: {1}},
	}

	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_surf"),
		Transitions: map[Edge]gameruntime.Transition{
			shore: {
				ID:         "surf_shore",
				Requires:   []gameruntime.CapabilityID{"can_surf"},
				PortBypass: true, // Surf: skipCanExit + relaxLanding
			},
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 3, 0, 0, 1, 1, nil, prereqs)
	if !errors.Is(err, ErrRouteReplanRequired) {
		t.Fatalf("Surf PortBypass error = %v, want ErrRouteReplanRequired", err)
	}
	if len(plan) != 1 || plan[0].Edge != shore {
		t.Fatalf("plan=%+v, want only the Surf semantic frontier", plan)
	}
	if plan[0].Transition == nil || plan[0].Transition.ID != "surf_shore" {
		t.Fatalf("first step transition = %+v, want surf_shore", plan[0].Transition)
	}
}

func TestSurfPortBypassPrefersReachableEquivalentBand(t *testing.T) {
	// Model Pallet Town's south edge after component segmentation: the first
	// band is an isolated two-tile pocket, while the second band is in the
	// player's ordinary land component. Both land in the same Route 21
	// component. PortBypass must widen reachability only when needed; it must
	// not let slice order choose the isolated pocket first.
	isolated := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirSouth, BandStart: 0, BandEnd: 1, BandScoped: true}
	reachable := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirSouth, BandStart: 2, BandEnd: 3, BandScoped: true}

	g := &Graph{
		componentAware: true,
		Edges: map[uint8][]Edge{
			1: {isolated, reachable}, // bad band deliberately first
			2: {},
		},
		comps: map[uint8][][]int{
			1: {{2, 2, 1, 1}},
			2: {{1, 1, 1, 1}},
		},
		tiles: map[uint8]dim{
			1: {w: 4, h: 1},
			2: {w: 4, h: 1},
		},
		exitComps: map[Edge][]int{
			isolated:  {2},
			reachable: {1},
		},
		entryComps: map[Edge][]int{
			isolated:  {1},
			reachable: {1},
		},
	}

	transition := gameruntime.Transition{
		ID:         "surf_shore",
		Requires:   []gameruntime.CapabilityID{"can_surf"},
		PortBypass: true,
	}
	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_surf"),
		Transitions: map[Edge]gameruntime.Transition{
			isolated:  transition,
			reachable: transition,
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 2, 0, 0, 0, nil, prereqs)
	if err != nil {
		t.Fatalf("route through equivalent Surf bands: %v", err)
	}
	if len(plan) != 1 || plan[0].Edge != reachable {
		t.Fatalf("plan=%+v, want reachable band %+v", plan, reachable)
	}
	if plan[0].Transition == nil || plan[0].Transition.ID != "surf_shore" {
		t.Fatalf("transition=%+v, want surf_shore", plan[0].Transition)
	}
}

func TestSurfPortBypassWithoutCapabilityStaysBlocked(t *testing.T) {
	shore := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirSouth, BandStart: 0, BandEnd: 3, BandScoped: true}

	g := &Graph{
		componentAware: true,
		Edges:          map[uint8][]Edge{1: {shore}, 2: {}},
		comps:          map[uint8][][]int{1: {{1}}, 2: {{1}}},
		tiles:          map[uint8]dim{1: {w: 1, h: 1}, 2: {w: 1, h: 1}},
	}
	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet(),
		Transitions: map[Edge]gameruntime.Transition{
			shore: {
				ID:         "surf_shore",
				Requires:   []gameruntime.CapabilityID{"can_surf"},
				PortBypass: true,
			},
		},
	}

	_, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 0, 0, 0, 0, nil, prereqs)
	var blocked *RouteBlockedError
	if !errors.As(err, &blocked) && !errors.Is(err, ErrNoRoute) {
		t.Fatalf("missing Surf capability error = %v, want RouteBlockedError or ErrNoRoute", err)
	}
}
