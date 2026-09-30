package skill

import (
	"sync"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// MoveObserver is told about every move Battle is about to press: the decoded
// turn and the final slot selected for execution. It observes only. It receives
// a copy of the battle state and no emulator, cannot change the slot, and runs
// without stepping a frame.
type MoveObserver func(b game.BattleState, executed int)

// MoveSelector is the narrow active-control seam. Battle first asks its normal
// deterministic MovePolicy, then gives that legal slot to the selector. The
// selector has no emulator/controller access; Battle validates the returned
// slot against the same usable set before any input is pressed.
type MoveSelector func(b game.BattleState, deterministic int) int

// BattleResultObserver receives the portable result at the exact battle exit
// boundary. Like MoveObserver it has no emulator and cannot affect inputs.
type BattleResultObserver func(result game.BattleResult)

var (
	scopedMoveObservers         sync.Map
	scopedMoveSelectors         sync.Map
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

// WithMoveSelector installs an active move selector for one emulator until
// the returned restore function is called. Nil preserves deterministic policy.
func WithMoveSelector(m *emu.Emu, selectMove MoveSelector) func() {
	if m == nil {
		return func() {}
	}
	previous, hadPrevious := scopedMoveSelectors.Load(m)
	if selectMove != nil {
		scopedMoveSelectors.Store(m, selectMove)
	} else {
		scopedMoveSelectors.Delete(m)
	}
	return func() {
		if hadPrevious {
			scopedMoveSelectors.Store(m, previous)
		} else {
			scopedMoveSelectors.Delete(m)
		}
	}
}

func selectMove(m *emu.Emu, b game.BattleState, deterministic int) int {
	raw, ok := scopedMoveSelectors.Load(m)
	if !ok {
		return deterministic
	}
	if selector, ok := raw.(MoveSelector); ok {
		return selector(b, deterministic)
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
