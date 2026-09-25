package skill

import (
	"sync"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// MoveObserver is told about every move Battle is about to press: the decoded
// turn and the slot the MovePolicy chose. It observes only. It receives a copy
// of the battle state and no emulator, cannot change the slot, and runs
// without stepping a frame, so an observed battle presses exactly the inputs
// on exactly the frames an unobserved one would (the RNG mixes in the cycle
// count, so frame neutrality is what keeps observation side-effect free).
type MoveObserver func(b state.BattleState, executed int)

var scopedMoveObservers sync.Map

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

func observeMove(m *emu.Emu, b state.BattleState, executed int) {
	raw, ok := scopedMoveObservers.Load(m)
	if !ok {
		return
	}
	if observe, ok := raw.(MoveObserver); ok {
		observe(b, executed)
	}
}
