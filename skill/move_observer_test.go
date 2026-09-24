package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

func TestWithMoveObserverIsScopedPerEmulatorAndRestores(t *testing.T) {
	a, b := &emu.Emu{}, &emu.Emu{}
	var outer, inner []int
	restoreOuter := WithMoveObserver(a, func(_ state.BattleState, slot int) { outer = append(outer, slot) })

	observeMove(a, state.BattleState{}, 1)
	observeMove(b, state.BattleState{}, 9)
	restoreInner := WithMoveObserver(a, func(_ state.BattleState, slot int) { inner = append(inner, slot) })
	observeMove(a, state.BattleState{}, 2)
	restoreInner()
	observeMove(a, state.BattleState{}, 3)
	restoreOuter()
	observeMove(a, state.BattleState{}, 4)

	if len(outer) != 2 || outer[0] != 1 || outer[1] != 3 {
		t.Fatalf("outer observer saw %v, want [1 3]", outer)
	}
	if len(inner) != 1 || inner[0] != 2 {
		t.Fatalf("inner observer saw %v, want [2]", inner)
	}
}

func TestMoveObserverGetsACopyOfTheTurn(t *testing.T) {
	m := &emu.Emu{}
	defer WithMoveObserver(m, func(b state.BattleState, _ int) { b.Moves[0].PP = 0 })()
	b := testGen1Snapshot().Battle
	pp := b.Moves[0].PP
	observeMove(m, b, 0)
	if b.Moves[0].PP != pp {
		t.Fatalf("observer mutated the battle turn: PP %d -> %d", pp, b.Moves[0].PP)
	}
}
