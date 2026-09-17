package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestNaturalMoveLearnerUsesWhichPokemonNotActiveBattleMon(t *testing.T) {
	var mem state.Mem
	mem[sym.PartyCount] = 2
	mem[sym.PlayerMonNumber] = 0
	mem[sym.WhichPokemon] = 1

	active := sym.PartyMon1
	mem[active+sym.MonType1] = 0x14
	mem[active+sym.MonType2] = 0x14
	copy(mem[active+sym.MonMoves:active+sym.MonMoves+4], []byte{10, 20, 0, 0})

	learner := sym.PartyMon1 + sym.PartyMonSize
	mem[learner+sym.MonType1] = 0x16
	mem[learner+sym.MonType2] = 0x03
	copy(mem[learner+sym.MonMoves:learner+sym.MonMoves+4], []byte{33, 45, 73, 22})

	mon, slot, ok := naturalMoveLearner(&mem, redWram())
	if !ok {
		t.Fatal("naturalMoveLearner reported no learner")
	}
	if slot != 1 {
		t.Fatalf("slot=%d want 1", slot)
	}
	if mon.Moves != [4]uint8{33, 45, 73, 22} {
		t.Fatalf("moves=%v want learner moves, not active battle moves", mon.Moves)
	}
	if mon.Type1 != 0x16 || mon.Type2 != 0x03 {
		t.Fatalf("types=(%#02x,%#02x) want learner types", mon.Type1, mon.Type2)
	}
}

func TestNaturalMoveLearnerRejectsInvalidWhichPokemon(t *testing.T) {
	var mem state.Mem
	mem[sym.PartyCount] = 1
	mem[sym.WhichPokemon] = 4
	if _, slot, ok := naturalMoveLearner(&mem, redWram()); ok || slot != 4 {
		t.Fatalf("slot=%d ok=%v want invalid slot 4", slot, ok)
	}
}
