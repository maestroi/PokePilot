package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	forcedMovementSettleBudget = 2400
	alwaysOnBikeBit            = 1 << 5
)

// forcedLandingForMap returns deterministic scripted movement triggered by
// entering a tile. The coordinate tables remain Red adapter facts; the local
// planner/executor is generic and sees only entry -> landing transitions.
func forcedLandingForMap(mapID uint8, x, y int) (world.Point, bool) {
	p := rocketPoint{x: x, y: y}
	var table map[rocketPoint]rocketPoint
	switch mapID {
	case rocketHideoutB2FMap, rocketHideoutB3FMap:
		table = rocketSpinnerTransitions(mapID)
	case viridianGymMap:
		table = viridianGymSpins
	default:
		return world.Point{}, false
	}
	landing, ok := table[p]
	if !ok {
		return world.Point{}, false
	}
	return world.Point{X: landing.x, Y: landing.y}, true
}

// cyclingRoadMoveAllowed models the irreversible downhill movement contract.
// The game sets BIT_ALWAYS_ON_BIKE when entering the Cycling Road corridor.
// Once set, north/uphill movement is not a legal navigation edge; the map-level
// route graph already applies the same one-way rule to Route 18->17 and
// Route 17->16 connections.
func cyclingRoadMoveAllowed(mem *state.Mem, mapID uint8, input world.Step) bool {
	if mem == nil || mem.U8(sym.StatusFlags6)&alwaysOnBikeBit == 0 {
		return true
	}
	switch mapID {
	case route16Map, route17Map, route18Map:
		return input != world.StepUp
	default:
		return true
	}
}

// executeForcedMovementStep presses the one input that enters a scripted
// movement tile and then lets the ROM own movement until the planned landing.
// It positively verifies map, coordinate, control, and walk-counter state.
func executeForcedMovementStep(m *emu.Emu, mapID uint8, input world.Step, landing world.Point) error {
	startX, startY := playerXY(m)
	btn, ok := buttonFor(input)
	if !ok {
		return fmt.Errorf("skill: forced movement: invalid input %s", input)
	}

	m.Press(btn)
	moved := false
	for i := 0; i < stepMoveBudget; i++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil {
			m.Release(btn)
			return ErrBattleInterrupted
		}
		if state.DecodeDialogue(&mem) != nil {
			m.Release(btn)
			return ErrDialogueInterrupted
		}
		x, y := playerXY(m)
		if x != startX || y != startY {
			moved = true
			break
		}
		m.StepFrame()
	}
	m.Release(btn)
	if !moved {
		return &ErrBlocked{Step: input, At: struct{ X, Y uint8 }{startX, startY}}
	}

	for i := 0; i < forcedMovementSettleBudget; i++ {
		if got := m.Peek8(sym.CurMap); got != mapID {
			return fmt.Errorf("skill: forced movement unexpectedly left map %#02x for %#02x", mapID, got)
		}
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil {
			return ErrBattleInterrupted
		}
		if state.DecodeDialogue(&mem) != nil {
			return ErrDialogueInterrupted
		}
		x, y := playerXY(m)
		if int(x) == landing.X && int(y) == landing.Y && state.Controllable(&mem) && mem.U8(sym.WalkCounter) == 0 {
			if err := waitForPositionStable(m, positionStableBudget, positionStableFrames); err != nil {
				return err
			}
			return nil
		}
		m.StepFrame()
	}
	x, y := playerXY(m)
	return fmt.Errorf("skill: forced movement did not settle at (%d,%d); ended at (%d,%d)", landing.X, landing.Y, x, y)
}

func normalizeForcedMovementError(err error) error {
	if errors.Is(err, ErrBattleInterrupted) {
		return ErrBattle
	}
	return err
}
