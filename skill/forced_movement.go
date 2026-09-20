package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const forcedMovementSettleBudget = 2400

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

// cyclingRoadAutoDown mirrors JoypadOverworld in Red. Route 17 is not a
// one-way map: explicit Up/Left/Right input is legal. The special mechanic is
// that, outside a trainer battle, *no held input* is replaced with PAD_DOWN.
// Navigation therefore has to prevent an input-release frame from becoming an
// unplanned south step between two deterministic path steps.
func cyclingRoadAutoDown(mapID uint8, trainerBattle, inputHeld bool) bool {
	return mapID == route17Map && !trainerBattle && !inputHeld
}

// stepOnceCyclingRoad performs one explicit Route 17 step while suppressing
// the ROM's no-input downhill coast during the settle window. B is the safe
// brake: JoypadOverworld treats it as held input, so it prevents the injected
// PAD_DOWN without changing position or interacting with the facing tile.
//
// The brake is pressed before the direction is released, meaning the ROM never
// observes a frame with no input between the requested movement and the settle.
// It is released only after WalkCounter/JoyIgnore settle; callers can then
// immediately press the next planned direction before advancing another frame.
func stepOnceCyclingRoad(m *emu.Emu, s world.Step, btn emu.Button) error {
	startX, startY := playerXY(m)
	targetX := int(startX) + s.DX
	targetY := int(startY) + s.DY

	m.Press(btn)
	moved := false
	for i := 0; i < 2*stepMoveBudget; i++ {
		x, y := playerXY(m)
		if int(x) == targetX && int(y) == targetY {
			moved = true
			break
		}
		if (x != startX || y != startY) && (int(x) != targetX || int(y) != targetY) {
			m.Release(btn)
			return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{startX, startY}}
		}
		m.StepFrame()
	}
	if !moved {
		m.Release(btn)
		return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{startX, startY}}
	}

	// Never expose a no-input frame here: that is exactly the condition Red
	// rewrites to PAD_DOWN on Route 17.
	m.Press(emu.B)
	m.Release(btn)
	for i := 0; i < stepSettleBudget; i++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil || state.DecodeDialogue(&mem) != nil {
			break
		}
		if mem.U8(sym.WalkCounter) == 0 && mem.U8(sym.JoyIgnore) == 0 {
			break
		}
		m.StepFrame()
	}
	m.Release(emu.B)

	x, y := playerXY(m)
	if int(x) != targetX || int(y) != targetY {
		return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{startX, startY}}
	}
	return nil
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
