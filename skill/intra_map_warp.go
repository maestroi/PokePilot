package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// intraMapWarpCrossBudget is deliberately larger than Traverse's ordinary
// map-flip budget. Warp pads run the teleport fade/spin animation and then
// reload the same map, so wCurMap never gives us the early positive edge that
// an ordinary door or stair does. The destination warp coordinate is the
// durable crossing predicate instead.
const intraMapWarpCrossBudget = 1200

// travelIntraMapWarpMaze is Travel for a destination whose disconnected rooms
// are joined by same-map warp pads. It reuses Travel's trainer/dialogue/blackout
// recovery; only the deterministic GoTo motor differs.
func travelIntraMapWarpMaze(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy, maxBattles int) (TravelResult, error) {
	if maxBattles <= 0 {
		return TravelResult{}, fmt.Errorf("skill: travelIntraMapWarpMaze: maxBattles must be > 0, got %d", maxBattles)
	}
	return travel(m, policy, maxBattles,
		func() error { return goToIntraMapWarpMaze(m, romData, dest) },
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fightOnly(m, policy),
	)
}

// goToIntraMapWarpMaze follows the component-aware route graph exactly like
// GoTo, but accepts a same-map warp as a real topology transition. The graph
// already knows which warp lands in which disconnected room; after every pad
// we throw away the rest of the route and plan again from the live coordinate.
func goToIntraMapWarpMaze(m *emu.Emu, romData []byte, dest Destination) error {
	g, err := world.BuildGraph(romData)
	if err != nil {
		return err
	}
	startX, startY := playerXY(m)
	start := navigationState{Map: m.Peek8(sym.CurMap), X: startX, Y: startY}
	if start.Map != dest.Map {
		return fmt.Errorf("skill: intra-map warp maze starts on map %02x, destination is map %02x", start.Map, dest.Map)
	}
	guard := newNavigationGuard(dest, start)

	for {
		if err := abortIfBattle(m); err != nil {
			return err
		}
		if err := waitOutScriptedMovement(m); err != nil {
			return err
		}
		cur := m.Peek8(sym.CurMap)
		x, y := playerXY(m)
		if cur != dest.Map {
			return fmt.Errorf("skill: intra-map warp maze unexpectedly left map %02x for %02x at (%d,%d)", dest.Map, cur, x, y)
		}

		h, err := rom.ParseMap(romData, cur)
		if err != nil {
			return fmt.Errorf("skill: intra-map warp maze parse map %02x: %w", cur, err)
		}
		liveGrid, err := liveMapGrid(m, romData, h)
		if err != nil {
			return fmt.Errorf("skill: intra-map warp maze build live map %02x: %w", cur, err)
		}
		routeGraph, err := g.WithMapGrid(cur, liveGrid)
		if err != nil {
			return fmt.Errorf("skill: intra-map warp maze overlay live map %02x: %w", cur, err)
		}
		route, err := world.FindRouteAtDestination(
			routeGraph, cur, dest.Map, int(x), int(y), int(dest.X), int(dest.Y), nil,
		)
		if err != nil {
			return fmt.Errorf("skill: intra-map warp maze no route on map %02x from (%d,%d) to (%d,%d): %w",
				cur, x, y, dest.X, dest.Y, err)
		}
		if len(route) == 0 {
			return walkWithinMap(m, romData, dest)
		}

		e := route[0]
		if e.Kind != world.EdgeWarp || e.From != cur || e.To != cur {
			return fmt.Errorf("skill: intra-map warp maze selected non-local edge %s", edgeName(e))
		}
		if err := traverseIntraMapWarp(m, romData, e); err != nil {
			return err
		}

		nowX, nowY := playerXY(m)
		if err := guard.observe(navigationState{Map: m.Peek8(sym.CurMap), X: nowX, Y: nowY}); err != nil {
			return fmt.Errorf("skill: intra-map warp maze: %w", err)
		}
	}
}

// intraMapWarpDestination returns the destination warp tile named by the
// selected source edge. Red stores DestWarpID as the zero-based index used by
// the route graph; for a same-map warp the destination lives in the same
// header. This is the positive identity used by traversal instead of relying
// on animation timing.
func intraMapWarpDestination(h rom.MapHeader, e world.Edge) (uint8, uint8, error) {
	if e.Kind != world.EdgeWarp || e.From != e.To {
		return 0, 0, fmt.Errorf("edge %s is not a same-map warp", edgeName(e))
	}
	for _, w := range h.Warps {
		if w.X != e.WarpX || w.Y != e.WarpY {
			continue
		}
		if w.DestMap != e.To {
			return 0, 0, fmt.Errorf("warp (%d,%d) on map %02x points to map %02x, want %02x",
				w.X, w.Y, e.From, w.DestMap, e.To)
		}
		if int(w.DestWarpID) >= len(h.Warps) {
			return 0, 0, fmt.Errorf("warp (%d,%d) on map %02x has destination index %d, only %d warps exist",
				w.X, w.Y, e.From, w.DestWarpID, len(h.Warps))
		}
		dest := h.Warps[w.DestWarpID]
		return dest.X, dest.Y, nil
	}
	return 0, 0, fmt.Errorf("source warp (%d,%d) not found on map %02x", e.WarpX, e.WarpY, e.From)
}

// traverseIntraMapWarp executes one warp-pad edge whose destination map is the
// source map. The ordinary Traverse contract watches for wCurMap to change;
// that can never happen here, so this variant verifies the exact destination
// warp coordinate, then waits for the teleport animation to return control.
func traverseIntraMapWarp(m *emu.Emu, romData []byte, e world.Edge) error {
	cur := m.Peek8(sym.CurMap)
	if cur != e.From || e.From != e.To || e.Kind != world.EdgeWarp {
		return fmt.Errorf("skill: traverseIntraMapWarp: invalid edge %s from current map %02x", edgeName(e), cur)
	}

	h, err := rom.ParseMap(romData, e.From)
	if err != nil {
		return fmt.Errorf("skill: traverseIntraMapWarp: parse map %02x: %w", e.From, err)
	}
	targetX, targetY, err := intraMapWarpDestination(h, e)
	if err != nil {
		return fmt.Errorf("skill: traverseIntraMapWarp: %w", err)
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return fmt.Errorf("skill: traverseIntraMapWarp: build live map %02x: %w", e.From, err)
	}

	var push world.Step
	var unwalkable error
	err = walkAround(func() error { return movementInterruption(m) }, func() map[[2]int]bool { return spriteBlockers(m) },
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			x, y := playerXY(m)
			_, _, steps, p, err := warpTarget(h, e, grid, int(x), int(y), blocked, romData)
			if err != nil {
				unwalkable = fmt.Errorf("skill: traverseIntraMapWarp: no reachable source pad from (%d,%d) on map %02x: %v: %w",
					x, y, e.From, err, ErrLegUnwalkable)
				return nil, unwalkable
			}
			push = p
			return steps, nil
		}, func(steps []world.Step) error { return WalkPath(m, steps) },
		func() { m.StepFrames(npcWaitFrames) })
	if err != nil {
		if err == unwalkable {
			return err
		}
		if errors.Is(err, ErrBattleInterrupted) {
			x, y := playerXY(m)
			return fmt.Errorf("skill: traverseIntraMapWarp: battle on map %02x at (%d,%d): %w", e.From, x, y, ErrBattle)
		}
		return fmt.Errorf("skill: traverseIntraMapWarp: walk to source pad: %w", err)
	}

	btn, ok := buttonFor(push)
	if !ok {
		return fmt.Errorf("skill: traverseIntraMapWarp: invalid push step %s", push)
	}
	m.Press(btn)
	crossed := false
	for i := 0; i < intraMapWarpCrossBudget; i++ {
		if got := m.Peek8(sym.CurMap); got != e.To {
			m.Release(btn)
			return fmt.Errorf("skill: traverseIntraMapWarp: warp %s unexpectedly changed map to %02x", edgeName(e), got)
		}
		x, y := playerXY(m)
		if x == targetX && y == targetY {
			crossed = true
			break
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			m.Release(btn)
			return fmt.Errorf("skill: traverseIntraMapWarp: battle while entering pad on map %02x: %w", e.From, ErrBattle)
		}
		m.StepFrame()
	}
	m.Release(btn)
	if !crossed {
		x, y := playerXY(m)
		return fmt.Errorf("skill: traverseIntraMapWarp: %s did not land on destination pad (%d,%d) within %d frames; at (%d,%d)",
			edgeName(e), targetX, targetY, intraMapWarpCrossBudget, x, y)
	}

	if _, err := m.StepUntil(arriveBudget, func(m *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return state.Controllable(&mem)
	}); err != nil {
		return fmt.Errorf("skill: traverseIntraMapWarp: player not controllable after landing on (%d,%d): %w", targetX, targetY, err)
	}
	if err := waitForPositionStable(m, positionStableBudget, positionStableFrames); err != nil {
		return fmt.Errorf("skill: traverseIntraMapWarp: %w", err)
	}
	if got := m.Peek8(sym.CurMap); got != e.To {
		return fmt.Errorf("skill: traverseIntraMapWarp: settled on map %02x, want %02x", got, e.To)
	}
	x, y := playerXY(m)
	if x != targetX || y != targetY {
		return fmt.Errorf("skill: traverseIntraMapWarp: settled at (%d,%d), want destination pad (%d,%d)", x, y, targetX, targetY)
	}
	return nil
}
