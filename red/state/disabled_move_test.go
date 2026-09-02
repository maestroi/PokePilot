package state

import (
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeBattleExcludesDisabledMove(t *testing.T) {
	var m Mem
	m[sym.IsInBattle] = 2
	for i := 0; i < 4; i++ {
		m[sym.BattleMonMoves+uint16(i)] = uint8(i + 1)
		m[sym.BattleMonPP+uint16(i)] = 10
	}

	// The ROM stores slot 2 in the high nibble and the remaining turn count
	// in the low nibble. Only the slot belongs in BattleState.DisabledMove.
	m[sym.PlayerDisabledMove] = 0x23

	b := DecodeBattle(&m)
	if b == nil {
		t.Fatal("DecodeBattle = nil, want trainer battle")
	}
	if b.DisabledMove != 2 {
		t.Fatalf("DisabledMove = %d, want 2", b.DisabledMove)
	}
	if got, want := b.Usable(), []int{0, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Usable() = %v, want %v: slot 1 (game slot 2) is disabled", got, want)
	}
}

func TestUsableIncludesMoveAgainWhenDisableClears(t *testing.T) {
	b := BattleState{DisabledMove: 2}
	for i := range b.Moves {
		b.Moves[i] = Move{ID: uint8(i + 1), PP: 10}
	}
	if got, want := b.Usable(), []int{0, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Usable() while disabled = %v, want %v", got, want)
	}

	b.DisabledMove = 0
	if got, want := b.Usable(), []int{0, 1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Usable() after disable clears = %v, want %v", got, want)
	}
}
