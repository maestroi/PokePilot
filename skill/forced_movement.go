package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/forcedmove"
	"github.com/maestroi/pokepilot/world"
)

const forcedMovementSettleBudget = 2400

// forcedLandingForMap is the adapter seam between portable local pathing and
// Red's script-derived trigger tables.
func forcedLandingForMap(mapID uint8, x, y int) (world.Point, bool) {
	landing, ok := forcedmove.Landing(mapID, x, y)
	if !ok {
		return world.Point{}, false
	}
	return world.Point{X: landing.X, Y: landing.Y}, true
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
func stepOnceCyclingRoad(m *emu.Emu, decoder game.OverworldDecoder, s world.Step, btn emu.Button) error {
	start := decoder.DecodeOverworld(m)
	startX, startY := start.X, start.Y
	targetX := int(startX) + s.DX
	targetY := int(startY) + s.DY

	m.Press(btn)
	moved := false
	for i := 0; i < 2*stepMoveBudget; i++ {
		live := decoder.DecodeOverworld(m)
		x, y := live.X, live.Y
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
		live := decoder.DecodeOverworld(m)
		if live.InBattle || live.InDialogue || live.MovementIdle {
			break
		}
		m.StepFrame()
	}
	m.Release(emu.B)

	live := decoder.DecodeOverworld(m)
	if int(live.X) != targetX || int(live.Y) != targetY {
		return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{startX, startY}}
	}
	return nil
}

// executeForcedMovementStep presses the one input that enters a scripted
// movement tile and then lets the ROM own movement until the planned landing.
// It positively verifies map, coordinate, control, and walk-counter state.
func executeForcedMovementStep(m *emu.Emu, decoder game.OverworldDecoder, mapID uint8, input world.Step, landing world.Point) error {
	start := decoder.DecodeOverworld(m)
	startX, startY := start.X, start.Y
	btn, ok := buttonFor(input)
	if !ok {
		return fmt.Errorf("skill: forced movement: invalid input %s", input)
	}

	m.Press(btn)
	moved := false
	for i := 0; i < stepMoveBudget; i++ {
		live := decoder.DecodeOverworld(m)
		if live.InBattle {
			m.Release(btn)
			return ErrBattleInterrupted
		}
		if live.InDialogue {
			m.Release(btn)
			return ErrDialogueInterrupted
		}
		if live.X != startX || live.Y != startY {
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
		live := decoder.DecodeOverworld(m)
		if live.NativeMapID != uint16(mapID) {
			return fmt.Errorf("skill: forced movement unexpectedly left map %#02x for %#04x", mapID, live.NativeMapID)
		}
		if live.InBattle {
			return ErrBattleInterrupted
		}
		if live.InDialogue {
			return ErrDialogueInterrupted
		}
		if int(live.X) == landing.X && int(live.Y) == landing.Y && live.Controllable && live.MovementIdle {
			if err := waitForPositionStableWithDecoder(m, decoder, positionStableBudget, positionStableFrames); err != nil {
				return err
			}
			return nil
		}
		m.StepFrame()
	}
	live := decoder.DecodeOverworld(m)
	return fmt.Errorf("skill: forced movement did not settle at (%d,%d); ended at (%d,%d)", landing.X, landing.Y, live.X, live.Y)
}

func normalizeForcedMovementError(err error) error {
	if errors.Is(err, ErrBattleInterrupted) {
		return ErrBattle
	}
	return err
}
