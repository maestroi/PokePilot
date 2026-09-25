package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

type fakePortableBattleMemory [8]byte

func (m *fakePortableBattleMemory) Peek8(addr uint16) byte { return m[addr] }
func (m *fakePortableBattleMemory) PeekInto(addr uint16, dst []byte) {
	copy(dst, m[int(addr):])
}

type fakeGen2BattleStateDecoder struct{}

func (fakeGen2BattleStateDecoder) DecodeBattleState(r game.MemoryReader) (game.BattleState, bool) {
	if r.Peek8(0) == 0 {
		return game.BattleState{}, false
	}
	return game.BattleState{
		Kind:          game.BattleTrainer,
		EnemySpecies:  251,
		ActiveSpecies: 250,
		EnemyType1:    0x1b,
		EnemyType2:    0x08,
		ActiveType1:   0x09,
		ActiveType2:   0x09,
		Moves: [4]game.BattleMove{
			{ID: 1, PP: 0},
			{ID: 251, PP: 5},
			{ID: 200, PP: 9, Disabled: true},
		},
		ActiveSpecialAttack:  91,
		ActiveSpecialDefense: 77,
	}, true
}

func (fakeGen2BattleStateDecoder) DecodeBattleResult(r game.MemoryReader) game.BattleResult {
	if r.Peek8(1) != 0 {
		return game.BattleLost
	}
	return game.BattleWon
}

func TestPortableMovePolicyAcceptsFakeGen2BattleState(t *testing.T) {
	mem := &fakePortableBattleMemory{}
	mem[0] = 1
	decoder := fakeGen2BattleStateDecoder{}

	b, ok := decodeBattleState(mem, decoder)
	if !ok {
		t.Fatal("fake Gen-II battle was not decoded")
	}
	if b.EnemySpecies != 251 || b.ActiveSpecialAttack != 91 || b.ActiveSpecialDefense != 77 {
		t.Fatalf("portable battle state was narrowed: %#v", b)
	}
	if got := FirstUsableMove(b); got != 1 {
		t.Fatalf("FirstUsableMove=%d want 1; zero-PP and disabled slots must be rejected", got)
	}
	mem[1] = 1
	if got := decoder.DecodeBattleResult(mem); got != game.BattleLost {
		t.Fatalf("result=%v want lost", got)
	}
}
