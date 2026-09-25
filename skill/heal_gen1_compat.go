package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// allPartyCenterRecovered is retained for Red-owned story/checkpoint callers
// and their historical regression tests. The reusable Heal transaction uses
// game.CenterDecoder instead.
func allPartyCenterRecovered(mem *state.Mem) bool {
	party := state.DecodeParty(mem)
	if party.Count == 0 {
		return false
	}
	for _, mon := range party.Mons {
		if mon.HP != mon.MaxHP || mon.Status != 0 {
			return false
		}
		for i, move := range mon.Moves {
			if move != 0 && mon.PP[i] == 0 {
				return false
			}
		}
	}
	return true
}

// settleHealBoundary is the legacy Red-memory boundary helper kept so existing
// Red tests and story code remain unchanged during the profile migration.
func settleHealBoundary(m frameClock, budget int) error {
	stable := 0
	final, _ := advanceCore(m, budget, func(mem *state.Mem) bool {
		if state.Controllable(mem) {
			stable++
			return stable >= healBoundaryStableFrames
		}
		stable = 0
		return false
	}, func(mem *state.Mem) bool {
		return state.DecodeTwoOptionMenu(mem) != nil || state.MenuUp(mem)
	})
	if stable >= healBoundaryStableFrames && state.Controllable(&final) {
		return nil
	}
	switch {
	case state.DecodeTwoOptionMenu(&final) != nil:
		return fmt.Errorf("skill: Heal: unexpected choice while settling completed heal")
	case state.MenuUp(&final):
		return fmt.Errorf("skill: Heal: unexpected menu while settling completed heal")
	default:
		return fmt.Errorf("skill: Heal: completed heal did not reach a stable controllable boundary within %d frames: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			budget, final.U8(sym.CurMap), final.U8(sym.XCoord), final.U8(sym.YCoord),
			final.U16BE(sym.JoyIgnore), final.U16BE(sym.FontLoaded))
	}
}

type healBoundaryMachine interface {
	frameClock
	game.MemoryReader
}

// settleHealBoundaryWithDecoder is retained for the #1888 regression tests.
// Production Heal now uses settlePokemonCenterBoundary, which is fully
// transaction-semantic as well as overworld-semantic.
func settleHealBoundaryWithDecoder(m healBoundaryMachine, decoder game.OverworldDecoder, budget int) error {
	stable := 0
	final, _ := advanceCore(m, budget, func(*state.Mem) bool {
		if decoder.DecodeOverworld(m).Controllable {
			stable++
			return stable >= healBoundaryStableFrames
		}
		stable = 0
		return false
	}, func(mem *state.Mem) bool {
		return state.DecodeTwoOptionMenu(mem) != nil || state.MenuUp(mem)
	})
	if stable >= healBoundaryStableFrames && decoder.DecodeOverworld(m).Controllable {
		return nil
	}
	switch {
	case state.DecodeTwoOptionMenu(&final) != nil:
		return fmt.Errorf("skill: Heal: unexpected choice while settling completed heal")
	case state.MenuUp(&final):
		return fmt.Errorf("skill: Heal: unexpected menu while settling completed heal")
	default:
		live, err := healRuntimeStateWithDecoder(m, decoder)
		if err != nil {
			return fmt.Errorf("skill: Heal: completed heal did not reach a stable controllable boundary within %d frames; observe world: %v", budget, err)
		}
		return fmt.Errorf("skill: Heal: completed heal did not reach a stable controllable boundary within %d frames: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			budget, live.Map, live.X, live.Y, final.U16BE(sym.JoyIgnore), final.U16BE(sym.FontLoaded))
	}
}
