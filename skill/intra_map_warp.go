package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// intraMapWarpCrossBudget is deliberately larger than Traverse's ordinary
// map-flip budget. Warp pads run the teleport fade/spin animation and then
// reload the same map, so wCurMap never gives us the early positive edge that
// an ordinary door or stair does. The destination warp coordinate is the
// durable crossing predicate instead.
const intraMapWarpCrossBudget = 1200

// intraMapWarpDestination returns the destination warp tile named by the
// selected source edge. Red stores DestWarpID as the zero-based index used by
// the route graph; for a same-map warp the destination lives in the same
// header. This is the positive identity used by traversal instead of relying
// on animation timing.
func intraMapWarpDestination(h worldmodel.HeaderView, e world.Edge) (uint8, uint8, error) {
	header := h.WorldMapHeader()
	if e.Kind != world.EdgeWarp || e.From != e.To {
		return 0, 0, fmt.Errorf("edge %s is not a same-map warp", edgeName(e))
	}
	for _, w := range header.Warps {
		if w.X != e.WarpX || w.Y != e.WarpY {
			continue
		}
		if w.DestMap != e.To {
			return 0, 0, fmt.Errorf("warp (%d,%d) on map %02x points to map %02x, want %02x",
				w.X, w.Y, e.From, w.DestMap, e.To)
		}
		if int(w.DestWarpID) >= len(header.Warps) {
			return 0, 0, fmt.Errorf("warp (%d,%d) on map %02x has destination index %d, only %d warps exist",
				w.X, w.Y, e.From, w.DestWarpID, len(header.Warps))
		}
		dest := header.Warps[w.DestWarpID]
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

	h, err := routingHeaderFor(m, e.From)
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
	err = walkAroundAvoidingObjects(func() error { return movementInterruption(m) }, m, h,
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			x, y := playerXY(m)
			_, _, steps, p, err := warpTarget(h, e, grid, int(x), int(y), blocked, nil, romData)
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
