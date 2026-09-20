package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
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
func fieldPathBridgeOnCurrentMap(
	m *emu.Emu,
	romData []byte,
	h rom.MapHeader,
	routeGraph *world.Graph,
	dest Destination,
	prereqs world.RoutePrerequisites,
	blockedHere map[world.Edge]bool,
) (Destination, bool, error) {
	cur := m.Peek8(sym.CurMap)
	if cur == dest.Map || routeGraph == nil {
		return Destination{}, false, nil
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return Destination{}, false, fmt.Errorf("skill: field-path bridge: live grid: %w", err)
	}
	sx, sy := playerXY(m)
	blocked := currentObservedStationaryObjectBlockers(m, h)
	blocked = warpAvoidance(h, int(sx), int(sy), blocked)

	type ranked struct {
		dest    Destination
		actions int
		moves   int
	}
	var best *ranked
	consider := func(x, y int) {
		if x < 0 || y < 0 || x > 255 || y > 255 {
			return
		}
		if !grid.Walkable(x, y) || blocked[[2]int{x, y}] {
			return
		}
		if x == int(sx) && y == int(sy) {
			return
		}
		bridge := Destination{Map: cur, X: uint8(x), Y: uint8(y)}
		plan, perr := currentFieldPathPlan(m, romData, h, bridge, blocked)
		if perr != nil {
			return
		}
		_, rerr := world.FindRoutePlanAtDestinationWithCapabilities(
			routeGraph, cur, dest.Map, x, y, int(dest.X), int(dest.Y), blockedHere, prereqs,
		)
		if rerr != nil {
			return
		}
		actions := 0
		for _, step := range plan {
			if step.Action != fieldPathWalk {
				actions++
			}
		}
		cand := ranked{dest: bridge, actions: actions, moves: len(plan)}
		if best == nil || cand.actions < best.actions || (cand.actions == best.actions && cand.moves < best.moves) {
			best = &cand
		}
	}

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
	if best == nil {
		return Destination{}, false, nil
	}
	return best.dest, true, nil
}
