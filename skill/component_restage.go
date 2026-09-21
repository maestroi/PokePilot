package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// componentRestagingDestination finds a standing tile ordinary routing can
// already reach and from which dest becomes reachable.
//
// Two shapes are covered:
//
//  1. Far-side landing of an outbound warp, when that landing (or one further
//     same-map gate hop from it) opens dest via FindRoutePlan or field-path
//     Cut/Surf bridging. This exits the Route 16 Fly house onto Route 16 west.
//  2. Same-map warp-adjacent tile needing at least one gate hop from the
//     standing pocket, from which dest opens the same way. This restages Route
//     16 west-north onto east-north so Cut can join the Celadon connection.
//
// Measured on run-2cs2fsg74sw2h31bfqwes80gq5: without this, nearestPokemonCenter
// / VirtualTrade die on world: no route from map 0xBC.
func componentRestagingDestination(
	m *emu.Emu,
	romData []byte,
	h rom.MapHeader,
	routeGraph *world.Graph,
	dest Destination,
	prereqs world.RoutePrerequisites,
	blockedHere map[world.Edge]bool,
) (Destination, bool, error) {
	if m == nil || routeGraph == nil {
		return Destination{}, false, nil
	}
	cur := m.Peek8(sym.CurMap)
	if cur == dest.Map {
		return Destination{}, false, nil
	}
	sx, sy := playerXY(m)

	type ranked struct {
		dest Destination
		hops int
	}
	var best *ranked
	consider := func(stage Destination, hops int) {
		if hops < 1 {
			return
		}
		if stage.Map == cur && int(stage.X) == int(sx) && int(stage.Y) == int(sy) {
			return
		}
		cand := ranked{dest: stage, hops: hops}
		if best == nil || cand.hops < best.hops ||
			(cand.hops == best.hops && (cand.dest.Map < best.dest.Map ||
				(cand.dest.Map == best.dest.Map && (cand.dest.Y < best.dest.Y ||
					(cand.dest.Y == best.dest.Y && cand.dest.X < best.dest.X))))) {
			best = &cand
		}
	}

	// (1) Exit onto a neighbour map landing that can open dest.
	for _, e := range routeGraph.Edges[cur] {
		if e.Kind != world.EdgeWarp {
			continue
		}
		landX, landY, ok := routeGraph.DestWarpTile(e)
		if !ok {
			continue
		}
		landH, err := rom.ParseMap(romData, e.To)
		if err != nil {
			continue
		}
		landGrid, err := gridForMap(m, romData, landH, e.To)
		if err != nil || landGrid == nil {
			continue
		}
		for _, d := range [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}, {0, 0}} {
			x, y := landX+d[0], landY+d[1]
			if x < 0 || y < 0 || x > 255 || y > 255 {
				continue
			}
			if !landGrid.Walkable(x, y) {
				continue
			}
			toStage, err := world.FindRoutePlanAtDestinationWithCapabilities(
				routeGraph, cur, e.To, int(sx), int(sy), x, y, blockedHere, prereqs,
			)
			if err != nil {
				continue
			}
			if !tileOpensDest(m, romData, routeGraph, e.To, x, y, dest, prereqs, blockedHere) {
				continue
			}
			hops := len(toStage)
			if hops == 0 {
				hops = 1
			}
			consider(Destination{Map: e.To, X: uint8(x), Y: uint8(y)}, hops)
			break
		}
	}

	// (2) Same-map gate restage onto another component of this map.
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return Destination{}, false, fmt.Errorf("skill: component restage: live grid: %w", err)
	}
	for _, w := range h.Warps {
		for _, d := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			x, y := int(w.X)+d[0], int(w.Y)+d[1]
			if !grid.Walkable(x, y) {
				continue
			}
			toStage, err := world.FindRoutePlanAtDestinationWithCapabilities(
				routeGraph, cur, cur, int(sx), int(sy), x, y, blockedHere, prereqs,
			)
			if err != nil || len(toStage) == 0 {
				continue
			}
			if !tileOpensDest(m, romData, routeGraph, cur, x, y, dest, prereqs, blockedHere) {
				continue
			}
			consider(Destination{Map: cur, X: uint8(x), Y: uint8(y)}, len(toStage))
		}
	}

	if best == nil {
		return Destination{}, false, nil
	}
	return best.dest, true, nil
}

func tileOpensDest(
	m *emu.Emu,
	romData []byte,
	routeGraph *world.Graph,
	mapID uint8,
	x, y int,
	dest Destination,
	prereqs world.RoutePrerequisites,
	blockedHere map[world.Edge]bool,
) bool {
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return false
	}
	grid, err := gridForMap(m, romData, h, mapID)
	if err != nil || grid == nil || !grid.Walkable(x, y) {
		return false
	}
	if _, err := world.FindRoutePlanAtDestinationWithCapabilities(
		routeGraph, mapID, dest.Map, x, y, int(dest.X), int(dest.Y), blockedHere, prereqs,
	); err == nil {
		return true
	}
	if _, ok, err := fieldPathBridgeFromTile(m, romData, h, routeGraph, grid, mapID, x, y, dest, prereqs, blockedHere); err == nil && ok {
		return true
	}
	// One nested same-map gate hop from this tile.
	for _, w := range h.Warps {
		for _, d := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := int(w.X)+d[0], int(w.Y)+d[1]
			if !grid.Walkable(nx, ny) || (nx == x && ny == y) {
				continue
			}
			toStage, err := world.FindRoutePlanAtDestinationWithCapabilities(
				routeGraph, mapID, mapID, x, y, nx, ny, blockedHere, prereqs,
			)
			if err != nil || len(toStage) == 0 {
				continue
			}
			if _, err := world.FindRoutePlanAtDestinationWithCapabilities(
				routeGraph, mapID, dest.Map, nx, ny, int(dest.X), int(dest.Y), blockedHere, prereqs,
			); err == nil {
				return true
			}
			if _, ok, err := fieldPathBridgeFromTile(m, romData, h, routeGraph, grid, mapID, nx, ny, dest, prereqs, blockedHere); err == nil && ok {
				return true
			}
		}
	}
	return false
}

func gridForMap(m *emu.Emu, romData []byte, h rom.MapHeader, mapID uint8) (*world.Grid, error) {
	if m.Peek8(sym.CurMap) == mapID {
		return liveMapGrid(m, romData, h)
	}
	provider, ok := worldmodel.ProviderForROM(romData)
	if !ok {
		return nil, fmt.Errorf("no map provider")
	}
	spec, err := provider.Grid(mapID, nil, worldmodel.TraversalLand)
	if err != nil {
		return nil, err
	}
	return world.GridFromSpec(spec)
}

func fieldPathBridgeFromTile(
	m *emu.Emu,
	romData []byte,
	h rom.MapHeader,
	routeGraph *world.Graph,
	grid *world.Grid,
	mapID uint8,
	sx, sy int,
	dest Destination,
	prereqs world.RoutePrerequisites,
	blockedHere map[world.Edge]bool,
) (Destination, bool, error) {
	if routeGraph == nil || mapID == dest.Map || grid == nil {
		return Destination{}, false, nil
	}
	blocked := map[[2]int]bool{}
	if m.Peek8(sym.CurMap) == mapID {
		blocked = currentObservedStationaryObjectBlockers(m, h)
	}
	blocked = warpAvoidance(h, sx, sy, blocked)

	var mem state.Mem
	state.Snapshot(m, &mem)
	caps := redRouteCapabilities(romData, &mem)
	canCut := caps.Has(capCanCut) || prereqs.Capabilities.Has(capCanCut)
	canSurf := caps.Has(capCanSurf) || prereqs.Capabilities.Has(capCanSurf)
	startWater := false
	rules := fieldPathRules{}
	var land, water fieldPathGrid = grid, grid
	if m.Peek8(sym.CurMap) == mapID {
		startWater = mem.U8(sym.WalkBikeSurfState) == fieldSurfingState
		landGrid, err := liveMapGridForTraversal(m, romData, h, world.TraversalLand)
		if err != nil {
			return Destination{}, false, err
		}
		waterGrid, err := liveMapGridForTraversal(m, romData, h, world.TraversalWater)
		if err != nil {
			return Destination{}, false, err
		}
		land, water = landGrid, waterGrid
		rules = currentFieldPathRules(m, h)
	} else {
		provider, ok := worldmodel.ProviderForROM(romData)
		if ok {
			if spec, err := provider.Grid(mapID, nil, worldmodel.TraversalLand); err == nil {
				if g, err := world.GridFromSpec(spec); err == nil {
					land = g
				}
			}
			if spec, err := provider.Grid(mapID, nil, worldmodel.TraversalWater); err == nil {
				if g, err := world.GridFromSpec(spec); err == nil {
					water = g
				}
			}
		}
	}

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
		if x == sx && y == sy {
			return
		}
		plan, perr := planFieldPath(
			land, water, h.Tileset,
			sx, sy, x, y,
			blocked,
			canCut, canSurf, startWater,
			rules,
		)
		if perr != nil {
			return
		}
		_, rerr := world.FindRoutePlanAtDestinationWithCapabilities(
			routeGraph, mapID, dest.Map, x, y, int(dest.X), int(dest.Y), blockedHere, prereqs,
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
		cand := ranked{dest: Destination{Map: mapID, X: uint8(x), Y: uint8(y)}, actions: actions, moves: len(plan)}
		if best == nil || cand.actions < best.actions || (cand.actions == best.actions && cand.moves < best.moves) {
			best = &cand
		}
	}

	for _, e := range routeGraph.Edges[mapID] {
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
