package skill

import (
	"errors"
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// fieldPathBridgeOnCurrentMap finds a same-map standing tile that local
// Cut/Surf pathing can reach and from which component routing can continue to
// dest. GoTo uses this when FindRoutePlan reports ErrNoRoute while the player
// is still stranded in a Cut-sealed pocket whose useful exits are ordinary
// warps/connections on this map.
//
// Same-map destinations already prefer field pathing directly. Cross-map
// destinations used to die on the static component split even when a single
// destination-aware Cut would open the needed port. Measured Celadon City
// south-of-gym pocket cases:
//   - Pokemon Center (catch / VirtualTrade): run-18zw4xby92x603chema3m8j2cm
//     and run-jjzpcm0bpqco24ijjvh3vm93q (triage ab11fbcf89382c39)
//   - Game Corner stand (silph_scope_acquired / RocketHideout):
//     run-3hksgfzyx8naz3kvcvmuevzjeo
//
// acceptBoundaryPrefix lets a bridge tile qualify when the policy route from
// it is a non-empty safe prefix ending at a semantic boundary
// (world.ErrRouteReplanRequired). Only GoTo's ErrNoRoute and RouteBlockedError
// branches set it: from a pocket with no executable route at all, any
// executable prefix is progress, and every destination behind a semantic gate
// only ever yields a prefix. Leaving
// Route 2's Diglett pocket for the Indigo lobby looped on "no route" until
// this was allowed (run-46xvnrqmqwcwvjnfqu5u03nj, issue #2006). The
// replan-required branch keeps it false so the Cinnabar ping-pong below
// cannot recur.
func fieldPathBridgeOnCurrentMap(
	m *emu.Emu,
	romData []byte,
	h worldmodel.HeaderView,
	routeGraph *world.Graph,
	dest Destination,
	prereqs world.RoutePrerequisites,
	blockedHere map[world.Edge]bool,
	acceptBoundaryPrefix bool,
) (Destination, bool, error) {
	if routeGraph == nil {
		return Destination{}, false, nil
	}
	live, err := currentRoutingRuntime(m)
	if err != nil {
		return Destination{}, false, err
	}
	if live.Map == dest.Map {
		return Destination{}, false, nil
	}
	search, err := newFieldPathBridgeSearch(m, romData, h, routeGraph)
	if err != nil {
		return Destination{}, false, err
	}
	for _, bridge := range search.bridges() {
		// Validate with the same travel-policy planner GoTo re-plans with from
		// the bridge tile. A policy-free check can accept a tile whose policy
		// route still stops at a semantic boundary, and that boundary's own
		// bridge leads back here. MEASURED on run-mis3rbm8t2283mwv0a6mrac8s:
		// fastest-policy GoTo from Cinnabar Island ping-ponged between
		// (6,4) and (6,10) until route_replan_exhausted.
		route, rerr := routePlanToDestinationByTravelPolicy(
			m, routeGraph, bridge.Map, int(bridge.X), int(bridge.Y), dest, blockedHere, prereqs,
		)
		if acceptBoundaryPrefix && errors.Is(rerr, world.ErrRouteReplanRequired) && len(route.Steps) > 0 {
			rerr = nil
		}
		if rerr == nil {
			return bridge, true, nil
		}
	}
	return Destination{}, false, nil
}

// fieldPathBridgeSearch is the destination-independent half of the bridge:
// the port-adjacent tiles on the current map and the local field-path state
// they are planned from, captured at one instant. RoutePlanner holds one so
// its offer-time answer accepts the same bridges GoTo would execute.
type fieldPathBridgeSearch struct {
	field   fieldPathSnapshot
	blocked map[[2]int]bool
	tiles   []Destination
}

func newFieldPathBridgeSearch(m *emu.Emu, romData []byte, h worldmodel.HeaderView, routeGraph *world.Graph) (*fieldPathBridgeSearch, error) {
	live, err := currentRoutingRuntime(m)
	if err != nil {
		return nil, err
	}
	cur, sx, sy := live.Map, int(live.X), int(live.Y)
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return nil, fmt.Errorf("skill: field-path bridge: live grid: %w", err)
	}
	field, err := captureFieldPath(m, romData, h)
	if err != nil {
		// No local field path means no bridge, not a routing failure.
		return &fieldPathBridgeSearch{}, nil
	}
	blocked := currentObservedStationaryObjectBlockers(m, h)
	blocked = warpAvoidance(h, sx, sy, blocked)

	search := &fieldPathBridgeSearch{field: field, blocked: blocked}
	seen := map[[2]int]bool{}
	consider := func(x, y int) {
		if x < 0 || y < 0 || x > 255 || y > 255 || seen[[2]int{x, y}] {
			return
		}
		seen[[2]int{x, y}] = true
		if !grid.Walkable(x, y) || blocked[[2]int{x, y}] || (x == sx && y == sy) {
			return
		}
		search.tiles = append(search.tiles, Destination{Map: cur, X: uint8(x), Y: uint8(y)})
	}
	if routeGraph != nil {
		for _, e := range routeGraph.Edges[cur] {
			switch e.Kind {
			case world.EdgeWarp:
				for _, w := range edgeWarpCandidates(h, e, romData) {
					for _, step := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
						consider(int(w.X)+step.DX, int(w.Y)+step.DY)
					}
				}
			case world.EdgeConnection:
				for _, at := range connectionFieldTargets(grid, e) {
					consider(at[0], at[1])
				}
			}
		}
	}
	return search, nil
}

// bridges lists the tiles local field pathing reaches with at least one field
// action, fewest actions then fewest moves first.
func (s *fieldPathBridgeSearch) bridges() []Destination {
	if s == nil {
		return nil
	}
	type ranked struct {
		dest    Destination
		actions int
		moves   int
	}
	var candidates []ranked
	for _, tile := range s.tiles {
		plan, _, err := s.field.plan(tile, s.blocked)
		if err != nil {
			continue
		}
		// This helper exists to bridge a topology split with a real local
		// field action. An ordinary zero-action walk must stay owned by the
		// global component/semantic router; otherwise it can pre-empt the
		// semantic action that actually opens the route. Farm #1492 and its
		// 0xCB siblings reproduced that shape in Rocket Hideout B1F: the
		// zero-action "bridge" steered back toward the elevator instead of
		// executing red:rocket_b1f_trainer_door.
		actions := fieldPathPlanActionCount(plan)
		if actions == 0 {
			continue
		}
		candidates = append(candidates, ranked{dest: tile, actions: actions, moves: len(plan)})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].actions != candidates[j].actions {
			return candidates[i].actions < candidates[j].actions
		}
		return candidates[i].moves < candidates[j].moves
	})
	out := make([]Destination, len(candidates))
	for i, c := range candidates {
		out[i] = c.dest
	}
	return out
}
