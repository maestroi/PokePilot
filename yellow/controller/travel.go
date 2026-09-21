package controller

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	travelReplanBudget = 24
	edgeCrossBudget    = 240
	arrivalBudget      = 900
)

// GoTo walks through Yellow's ROM-derived map graph to one concrete native
// destination. It handles ordinary warps and border connections only. Story
// pivots (Cut/Surf/Strength/Poké Flute etc.) remain separate semantic
// controllers; if one blocks a leg this returns a typed, bounded error for the
// objective runtime to replan around rather than importing Red's gate logic.
func GoTo(m *emu.Emu, romData []byte, destMap, destX, destY uint8) error {
	if m == nil {
		return fmt.Errorf("yellow travel: nil emulator")
	}
	graph, err := world.BuildGraph(romData)
	if err != nil {
		return fmt.Errorf("yellow travel: build graph: %w", err)
	}

	flyAttempted := false
	for attempt := 0; attempt < travelReplanBudget; attempt++ {
		recovered, err := recoverYellowTravelInterruption(m, romData)
		if err != nil {
			return err
		}
		if recovered {
			continue
		}

		cur := m.Peek8(sym.CurMap)
		x, y := m.Peek8(sym.XCoord), m.Peek8(sym.YCoord)
		if cur == destMap && x == destX && y == destY {
			return nil
		}

		// Prefer Fly for already-visited city destinations once Yellow can
		// actually prepare/use HM02. Only outdoor maps can open Fly. An
		// unvisited destination is a normal routing fallback, not a failure.
		if !flyAttempted && cur < 0x25 && destMap < yellowFlyCityCount && cur != destMap {
			cap := fieldCapabilityFor(m, romData, FieldFly)
			if cap.Usable || cap.Preparable {
				flyAttempted = true
				if err := FlyTo(m, romData, destMap); err == nil {
					continue
				} else if !errors.Is(err, ErrFlyDestinationUnvisited) {
					return fmt.Errorf("yellow travel: Fly to %#02x: %w", destMap, err)
				}
			}
		}

		route, err := world.FindRouteAtDestination(
			graph, cur, destMap, int(x), int(y), int(destX), int(destY), nil,
		)
		if err != nil {
			return fmt.Errorf("yellow travel: route %#02x (%d,%d) -> %#02x (%d,%d): %w",
				cur, x, y, destMap, destX, destY, err)
		}
		if len(route) == 0 {
			if err := walkTo(m, romData, int(destX), int(destY), nil); err != nil {
				recovered, recoverErr := recoverYellowTravelInterruption(m, romData)
				if recoverErr != nil {
					return fmt.Errorf("yellow travel: final walk recovery on map %#02x: %w", cur, recoverErr)
				}
				if recovered {
					continue
				}
				return fmt.Errorf("yellow travel: final walk on map %#02x: %w", cur, err)
			}
			continue
		}

		if err := traverseYellowEdge(m, romData, route[0]); err != nil {
			recovered, recoverErr := recoverYellowTravelInterruption(m, romData)
			if recoverErr != nil {
				return fmt.Errorf("yellow travel: edge recovery: %w", recoverErr)
			}
			if recovered {
				continue
			}
			return err
		}
	}
	return fmt.Errorf("yellow travel: exceeded %d re-plans toward map %#02x (%d,%d)",
		travelReplanBudget, destMap, destX, destY)
}

func recoverYellowTravelInterruption(m *emu.Emu, romData []byte) (bool, error) {
	if m.Peek8(sym.IsInBattle) != 0 {
		result, err := Battle(m, romData)
		if err != nil {
			return true, fmt.Errorf("yellow travel: resolve battle: %w", err)
		}
		if result.Outcome == BattleOutcomeLost {
			// Blackout recovery is a valid new routing origin. The next loop
			// replans from the semantic respawn state rather than pretending
			// the interrupted leg still applies.
			return true, nil
		}
		return true, nil
	}

	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return false, fmt.Errorf("yellow travel: observe interruption: %w", err)
	}
	if m.Peek8(sym.FontLoaded) != 0 || !obs.Controllable {
		if _, err := RecoverDialogue(m, romData); err != nil {
			return true, fmt.Errorf("yellow travel: resolve dialogue/cutscene: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func traverseYellowEdge(m *emu.Emu, romData []byte, edge world.Edge) error {
	if got := m.Peek8(sym.CurMap); got != edge.From {
		return fmt.Errorf("yellow travel: edge starts on %#02x but player is on %#02x", edge.From, got)
	}
	h, err := yellowrom.ParseMap(romData, edge.From)
	if err != nil {
		return fmt.Errorf("yellow travel: parse map %#02x: %w", edge.From, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return fmt.Errorf("yellow travel: build grid %#02x: %w", edge.From, err)
	}
	blocked := staticObjectBlockers(h, nil)

	var push world.Step
	switch edge.Kind {
	case world.EdgeConnection:
		tx, ty, p, err := yellowConnectionTarget(grid, edge, int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)), blocked)
		if err != nil {
			return fmt.Errorf("yellow travel: connection %#02x->%#02x: %w", edge.From, edge.To, err)
		}
		if err := walkTo(m, romData, tx, ty, nil); err != nil {
			return fmt.Errorf("yellow travel: approach connection %#02x->%#02x: %w", edge.From, edge.To, err)
		}
		push = p

	case world.EdgeWarp:
		target := [2]int{int(edge.WarpX), int(edge.WarpY)}
		for _, warp := range h.Warps {
			p := [2]int{int(warp.X), int(warp.Y)}
			if p != target {
				blocked[p] = true
			}
		}
		sx, sy := int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord))
		steps, p, err := world.FindPathAdjacent(grid, sx, sy, target[0], target[1], blocked)
		if err != nil {
			return fmt.Errorf("yellow travel: no path to warp %#02x->%#02x at (%d,%d): %w",
				edge.From, edge.To, edge.WarpX, edge.WarpY, err)
		}
		if err := walkPath(m, edge.From, steps); err != nil {
			return fmt.Errorf("yellow travel: approach warp %#02x->%#02x: %w", edge.From, edge.To, err)
		}
		push = p

	default:
		return fmt.Errorf("yellow travel: unknown edge kind %d", edge.Kind)
	}

	btn, ok := buttonFor(push)
	if !ok {
		return fmt.Errorf("yellow travel: invalid edge push %+v", push)
	}
	m.Press(btn)
	crossed := false
	for i := 0; i < edgeCrossBudget; i++ {
		if m.Peek8(sym.CurMap) != edge.From {
			crossed = true
			break
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			m.Release(btn)
			return fmt.Errorf("yellow travel: battle while crossing %#02x->%#02x", edge.From, edge.To)
		}
		m.StepFrame()
	}
	m.Release(btn)
	if !crossed {
		return fmt.Errorf("yellow travel: edge %#02x->%#02x did not cross within %d frames",
			edge.From, edge.To, edgeCrossBudget)
	}

	if _, err := m.StepUntil(arrivalBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.CurMap) == edge.To &&
			m.Peek8(sym.CurMapWidth) != 0 &&
			m.Peek8(sym.CurMapHeight) != 0 &&
			m.Peek8(sym.FontLoaded) == 0 &&
			m.Peek8(sym.JoyIgnore) == 0 &&
			m.Peek8(sym.WalkCounter) == 0
	}); err != nil {
		return fmt.Errorf("yellow travel: arrival %#02x->%#02x did not settle: %w", edge.From, edge.To, err)
	}
	return nil
}

func yellowConnectionTarget(grid *world.Grid, edge world.Edge, sx, sy int, blocked map[[2]int]bool) (int, int, world.Step, error) {
	if grid == nil {
		return 0, 0, world.Step{}, fmt.Errorf("nil grid")
	}
	start, end := 0, grid.Width-1
	if edge.Dir >= 2 {
		end = grid.Height - 1
	}
	if edge.BandScoped {
		start, end = int(edge.BandStart), int(edge.BandEnd)
	}

	bestX, bestY, bestLen := -1, -1, -1
	for i := start; i <= end; i++ {
		tx, ty := i, 0
		switch edge.Dir {
		case 0:
			tx, ty = i, 0
		case 1:
			tx, ty = i, grid.Height-1
		case 2:
			tx, ty = 0, i
		case 3:
			tx, ty = grid.Width-1, i
		default:
			return 0, 0, world.Step{}, fmt.Errorf("unknown connection direction %d", edge.Dir)
		}
		if !grid.Walkable(tx, ty) || blocked[[2]int{tx, ty}] {
			continue
		}
		steps, err := world.FindPath(grid, sx, sy, tx, ty, blocked)
		if err != nil {
			continue
		}
		if bestLen < 0 || len(steps) < bestLen {
			bestX, bestY, bestLen = tx, ty, len(steps)
		}
	}
	if bestLen < 0 {
		return 0, 0, world.Step{}, fmt.Errorf("no reachable border tile in connection band")
	}
	return bestX, bestY, connectionPush(edge.Dir), nil
}

func connectionPush(dir uint8) world.Step {
	switch dir {
	case 0:
		return world.StepUp
	case 1:
		return world.StepDown
	case 2:
		return world.StepLeft
	case 3:
		return world.StepRight
	default:
		return world.Step{}
	}
}
