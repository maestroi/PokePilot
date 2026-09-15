package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestLearningMonIsWhichPokemonNotActiveMon is the deterministic regression for
// the farm defect "natural move 77 was accepted for party slot 0 ... but the
// resulting move set was never verified" (run-2cp4p8pqm4c6d1d0rpp7i1wyxt).
//
// GainExperience offers a level-up move to every mon with the gain-exp flag
// set, naming the one being processed in wWhichPokemon — which is not
// necessarily the active mon (wPlayerMonNumber). The learn decision and the
// end-of-battle verification must both target the wWhichPokemon slot. This
// asserts learningMon returns that slot, not the active one: reverting it to
// wPlayerMonNumber makes this fail.
func TestLearningMonIsWhichPokemonNotActiveMon(t *testing.T) {
	var mem state.Mem
	mem[sym.PartyCount] = 2

	// Active mon in slot 0: the one out in battle.
	base0 := sym.PartyMon1
	mem[base0+sym.MonSpecies] = 1
	putU16BE(&mem, base0+sym.MonMaxHP, 74)
	putU16BE(&mem, base0+sym.MonHP, 42)
	mem[base0+sym.MonMoves] = 16

	// Learning mon in slot 1: benched, but the one offered the move.
	base1 := sym.PartyMon1 + sym.PartyMonSize
	mem[base1+sym.MonSpecies] = 2
	putU16BE(&mem, base1+sym.MonMaxHP, 67)
	putU16BE(&mem, base1+sym.MonHP, 36)
	mem[base1+sym.MonMoves] = 33

	mem[sym.PlayerMonNumber] = 0 // active is slot 0
	mem[sym.WhichPokemon] = 1    // learning is slot 1

	lm, ok := learningMon(&mem)
	if !ok {
		t.Fatal("learningMon = false, want true")
	}
	if lm.Species != 2 {
		t.Fatalf("learningMon species = %d, want 2 (the wWhichPokemon slot, not the active slot 0)", lm.Species)
	}
}
