package skill

import (
	"sync"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// MoveObserver is told about every move Battle is about to press: the decoded
// turn and the slot the MovePolicy chose. It observes only. It receives a copy
// of the battle state and no emulator, cannot change the slot, and runs
// without stepping a frame, so an observed battle presses exactly the inputs
// on exactly the frames an unobserved one would (the RNG mixes in the cycle
// count, so frame neutrality is what keeps observation side-effect free).
type MoveObserver func(b game.BattleState, executed int)

// BattleMoveController may replace the deterministic move slot at the final
// move-selection boundary. It receives only a copy of portable battle state
// plus the already-legal deterministic slot; it has no emulator/controller
// access. Returning the deterministic slot is always a safe fallback.
type BattleMoveController func(b game.BattleState, deterministic int) int

// BattleResultObserver receives the portable result at the exact battle exit
// boundary. Like MoveObserver it has no emulator and cannot affect inputs.
type BattleResultObserver func(result game.BattleResult)

var (
	scopedMoveObservers         sync.Map
	scopedBattleMoveControllers sync.Map
	scopedBattleResultObservers sync.Map
)

// WithMoveObserver installs observe for one emulator until the returned
// restore function is called. No observer is the historical behavior, which
// every direct skill caller keeps.
func WithMoveObserver(m *emu.Emu, observe MoveObserver) func() {
	if m == nil {
		return func() {}
	}
	previous, hadPrevious := scopedMoveObservers.Load(m)
	if observe != nil {
		scopedMoveObservers.Store(m, observe)
	} else {
		scopedMoveObservers.Delete(m)
	}
	return func() {
		if hadPrevious {
			scopedMoveObservers.Store(m, previous)
		} else {
			scopedMoveObservers.Delete(m)
		}
	}
}

// WithBattleMoveController installs a move controller for one emulator.
func WithBattleMoveController(m *emu.Emu, control BattleMoveController) func() {
	if m == nil {
		return func() {}
	}
	previous, hadPrevious := scopedBattleMoveControllers.Load(m)
	if control != nil {
		scopedBattleMoveControllers.Store(m, control)
	} else {
		scopedBattleMoveControllers.Delete(m)
	}
	return func() {
		if hadPrevious {
			scopedBattleMoveControllers.Store(m, previous)
		} else {
			scopedBattleMoveControllers.Delete(m)
		}
	}
}

func controlBattleMove(m *emu.Emu, b game.BattleState, deterministic int) int {
	raw, ok := scopedBattleMoveControllers.Load(m)
	if !ok {
		return deterministic
	}
	if control, ok := raw.(BattleMoveController); ok {
		return control(b, deterministic)
	}
	return deterministic
}

// WithBattleResultObserver installs a battle-exit observer for one emulator.
func WithBattleResultObserver(m *emu.Emu, observe BattleResultObserver) func() {
	if m == nil {
		return func() {}
	}
	previous, hadPrevious := scopedBattleResultObservers.Load(m)
	if observe != nil {
		scopedBattleResultObservers.Store(m, observe)
	} else {
		scopedBattleResultObservers.Delete(m)
	}
	return func() {
		if hadPrevious {
			scopedBattleResultObservers.Store(m, previous)
		} else {
			scopedBattleResultObservers.Delete(m)
		}
	}
}

func observeBattleResult(m *emu.Emu, result game.BattleResult) {
	raw, ok := scopedBattleResultObservers.Load(m)
	if !ok {
		return
	}
	if observe, ok := raw.(BattleResultObserver); ok {
		observe(result)
	}
}

func observeMove(m *emu.Emu, b game.BattleState, executed int) {
	raw, ok := scopedMoveObservers.Load(m)
	if !ok {
		return
	}
	if observe, ok := raw.(MoveObserver); ok {
		observe(b, executed)
	}
}
