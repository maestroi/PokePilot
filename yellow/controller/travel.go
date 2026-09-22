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
// destination. Local and edge approaches re-plan through Yellow-owned Cut,
// Surf, Strength and Flash mechanics when the current party can use or prepare
// them. Story pivots such as Snorlax and scripted puzzle events remain separate
// semantic controllers; navigation never guesses through those gates.
func GoTo(m *emu.Emu, romData []byte, destMap, destX, destY uint8) error {
	if m == nil {
		return fmt.Errorf("yellow travel: nil emulator")
	}
	graph, err := world.BuildGraph(romData)
	if err != nil {
		return fmt.Errorf("yellow travel: build graph: %w", err)
	}

	flyAttempted := false
	blockedHere := map[world.Edge]bool{}
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
		if err := yellowMaybeFlash(m, romData); err != nil {
			return err
		}
		if cur == destMap {
			err := yellowWalkToWithFieldActions(m, romData, int(destX), int(destY), nil)
			if err == nil {
				continue
			}
			recovered, recoverErr := recoverYellowTravelInterruption(m, romData)
			if recoverErr != nil {
				return fmt.Errorf("yellow travel: local field-path recovery on map %#02x: %w", cur, recoverErr)
			}
			if recovered {
				continue
			}
			if !errors.Is(err, world.ErrNoPath) {
				return fmt.Errorf("yellow travel: local field path on map %#02x: %w", cur, err)
			}
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
			graph, cur, destMap, int(x), int(y), int(destX), int(destY), blockedHere,
		)
		if err != nil && cur != destMap && errors.Is(err, world.ErrNoRoute) {
			// The static land-component graph cannot represent a first hop
			// whose port is reached by Cut/Surf/Strength. Fall back to map-level
			// routing for that first hop; traverseYellowEdge still proves the
			// concrete field-aware approach before crossing.
			route, err = world.FindRouteAvoiding(graph, cur, destMap, blockedHere)
		}
		if err != nil {
			return fmt.Errorf("yellow travel: route %#02x (%d,%d) -> %#02x (%d,%d): %w",
				cur, x, y, destMap, destX, destY, err)
		}
		if len(route) == 0 {
			return fmt.Errorf("yellow travel: no local field path or map transition from %#02x toward (%d,%d)",
				cur, destX, destY)
		}

		if err := traverseYellowEdge(m, romData, route[0]); err != nil {
			recovered, recoverErr := recoverYellowTravelInterruption(m, romData)
			if recoverErr != nil {
				return fmt.Errorf("yellow travel: edge recovery: %w", recoverErr)
			}
			if recovered {
				continue
			}
			if errors.Is(err, world.ErrNoPath) {
				blockedHere[route[0]] = true
				continue
			}
			return err
		}
		blockedHere = map[world.Edge]bool{}
	}
	return fmt.Errorf("yellow travel: exceeded %d re-plans toward map %#02x (%d,%d)",
		travelReplanBudget, destMap, destX, destY)
}

func recoverYellowTravelInterruption(m *emu.Emu, romData []byte) (bool, error) {
	if m.Peek8(sym.IsInBattle) != 0 {
		if m.Peek8(sym.BattleType) == 2 {
			if err := FleeSafari(m, romData); err != nil {
				return true, fmt.Errorf("yellow travel: flee Safari encounter: %w", err)
			}
			return true, nil
		}
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

	var push world.Step
	switch edge.Kind {
	case world.EdgeConnection:
		tx, ty, p, err := yellowConnectionFieldTarget(m, romData, h, edge)
		if err != nil {
			return fmt.Errorf("yellow travel: connection %#02x->%#02x: %w", edge.From, edge.To, err)
		}
		if err := yellowWalkToWithFieldActions(m, romData, tx, ty, nil); err != nil {
			return fmt.Errorf("yellow travel: approach connection %#02x->%#02x: %w", edge.From, edge.To, err)
		}
		push = p

	case world.EdgeWarp:
		tx, ty, p, blocked, err := yellowWarpFieldApproach(m, romData, h, edge)
		if err != nil {
			return fmt.Errorf("yellow travel: no path to warp %#02x->%#02x at (%d,%d): %w",
				edge.From, edge.To, edge.WarpX, edge.WarpY, err)
		}
		if err := yellowWalkToWithFieldActions(m, romData, tx, ty, blocked); err != nil {
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

func yellowLocalFieldReachCost(m *emu.Emu, romData []byte, h yellowrom.MapHeader, tx, ty int, extra map[[2]int]bool) (int, bool, error) {
	plan, err := yellowCurrentFieldPath(m, romData, h, tx, ty, extra)
	if err == nil {
		actions := 0
		for _, step := range plan {
			if step.Action != yellowFieldWalk {
				actions++
			}
		}
		return actions*10000 + len(plan), true, nil
	}
	if !errors.Is(err, world.ErrNoPath) {
		return 0, false, err
	}
	strength, needed, err := yellowStrengthPlan(m, romData, h, tx, ty, extra)
	if err != nil {
		return 0, false, err
	}
	if !needed {
		return 0, false, nil
	}
	cap := fieldCapabilityFor(m, romData, FieldStrength)
	if !cap.Usable && !cap.Preparable {
		return 0, false, nil
	}
	return 20000 + len(strength.Pushes)*100 + len(strength.FinalWalk), true, nil
}

func yellowConnectionFieldTarget(m *emu.Emu, romData []byte, h yellowrom.MapHeader, edge world.Edge) (int, int, world.Step, error) {
	grid, err := yellowLiveMapGridForTraversal(m, romData, h, world.TraversalLand)
	if err != nil {
		return 0, 0, world.Step{}, err
	}
	start, end := 0, grid.Width-1
	if edge.Dir >= 2 {
		end = grid.Height - 1
	}
	if edge.BandScoped {
		start, end = int(edge.BandStart), int(edge.BandEnd)
	}
	bestX, bestY, bestCost := -1, -1, -1
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
		cost, ok, err := yellowLocalFieldReachCost(m, romData, h, tx, ty, nil)
		if err != nil {
			return 0, 0, world.Step{}, err
		}
		if !ok {
			continue
		}
		if bestCost < 0 || cost < bestCost {
			bestX, bestY, bestCost = tx, ty, cost
		}
	}
	if bestCost < 0 {
		return 0, 0, world.Step{}, world.ErrNoPath
	}
	return bestX, bestY, connectionPush(edge.Dir), nil
}

func yellowWarpFieldApproach(m *emu.Emu, romData []byte, h yellowrom.MapHeader, edge world.Edge) (int, int, world.Step, map[[2]int]bool, error) {
	targetX, targetY := int(edge.WarpX), int(edge.WarpY)
	extra := map[[2]int]bool{}
	for _, warp := range h.Warps {
		if int(warp.X) == targetX && int(warp.Y) == targetY {
			continue
		}
		extra[[2]int{int(warp.X), int(warp.Y)}] = true
	}

	bestX, bestY, bestCost := -1, -1, -1
	var bestPush world.Step
	for _, push := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		standX, standY := targetX-push.DX, targetY-push.DY
		cost, ok, err := yellowLocalFieldReachCost(m, romData, h, standX, standY, extra)
		if err != nil {
			return 0, 0, world.Step{}, nil, err
		}
		if !ok {
			continue
		}
		if bestCost < 0 || cost < bestCost {
			bestX, bestY, bestCost, bestPush = standX, standY, cost, push
		}
	}
	if bestCost < 0 {
		return 0, 0, world.Step{}, extra, world.ErrNoPath
	}
	return bestX, bestY, bestPush, extra, nil
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
