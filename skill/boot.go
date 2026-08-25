package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// atControllableOverworld is the single success predicate for the boot: the
// player is on Red's bedroom map (CurMap == 0x26) and the game accepts free
// overworld input.
func atControllableOverworld(m *state.Mem) bool {
	return m.U8(sym.CurMap) == 0x26 && state.Controllable(m)
}

// BootToOverworld drives a fresh Pokemon Red from power-on to a controllable
// overworld, verifying the end state from RAM rather than pixels.
//
// The sequence, measured on this ROM:
//  1. StepFrames(300) to let the game boot.
//  2. Loop up to 900 iterations: tap Start (hold 3, gap 7) for the first 4
//     iterations to clear the title screen, then tap A for the rest. Plain
//     A-tapping gets through the name-selection screens on this ROM. After
//     each tap, check the predicate and return as soon as it holds. The real
//     overworld is reached at roughly frame 3310, around iteration 300.
//
// It returns the decoded game state at the overworld, or an error naming the
// last decoded state if the overworld is not reached within budget.
func BootToOverworld(m *emu.Emu) (state.GameState, error) {
	var mem state.Mem

	m.StepFrames(300)

	const budget = 900
	for i := 0; i < budget; i++ {
		if i < 4 {
			m.Tap(emu.Start, 3, 7)
		} else {
			m.Tap(emu.A, 3, 7)
		}
		state.Snapshot(m, &mem)
		if atControllableOverworld(&mem) {
			return state.Decode(&mem), nil
		}
	}

	// Timeout: report the last decoded state so a regression is diagnosable.
	// CurMapWidth/CurMapHeight are included because a zero-dimension map is the
	// signature of the intro still running (the map was never actually loaded).
	state.Snapshot(m, &mem)
	last := state.Decode(&mem)
	menuOpen := "no"
	if mem.U8(sym.FontLoaded) != 0 {
		menuOpen = fmt.Sprintf("yes (cur=%d max=%d)", last.Menu.Current, last.Menu.Max)
	}
	return state.GameState{}, fmt.Errorf(
		"boot: no controllable overworld within %d iterations; last: map=%#04x x=%d y=%d mapW=%d mapH=%d fontLoaded=%#04x menu=%s controllable=%v",
		budget, last.Player.MapID, last.Player.X, last.Player.Y,
		last.World.Width, last.World.Height,
		mem.U8(sym.FontLoaded), menuOpen, state.Controllable(&mem))
}
