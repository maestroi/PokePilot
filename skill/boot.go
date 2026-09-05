package skill

import (
	"fmt"
	"strings"

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

// introNameMenu reports whether Oak's player/rival preset-name menu is on
// screen. Both menus have NEW NAME at index 0 followed by three built-in
// presets. Detect the menu from live RAM/text rather than frame timing so boot
// never falls through into the naming keyboard just because the intro took a
// slightly different number of frames.
func introNameMenu(m *state.Mem) bool {
	if m.U8(sym.FontLoaded) == 0 || m.U8(sym.MaxMenuItem) != 3 {
		return false
	}
	return strings.Contains(state.ScreenText(m), "NEW NAME")
}

// bootInput chooses the next deterministic input for the fresh-game intro.
// The first four Start taps preserve the existing title/menu skip. Once a
// player/rival name menu appears, steer to index 1 and select it: Pokemon Red's
// first presets are RED and BLUE. Any other intro state is ordinary dialogue,
// where A is the safe paging input.
func bootInput(m *state.Mem, iteration int) emu.Button {
	if iteration < 4 {
		return emu.Start
	}
	if !introNameMenu(m) {
		return emu.A
	}

	switch m.U8(sym.CurrentMenuItem) {
	case 0:
		return emu.Down
	case 1:
		return emu.A
	default:
		return emu.Up
	}
}

// BootToOverworld drives a fresh Pokemon Red from power-on to a controllable
// overworld, verifying the end state from RAM rather than pixels.
//
// The sequence, measured on this ROM:
//  1. StepFrames(300) to let the game boot.
//  2. Loop up to 900 iterations. The first 4 iterations tap Start to clear the
//     title/menu. Ordinary intro dialogue taps A. When Oak's player or rival
//     name menu appears, detect it from RAM/text, move to preset index 1 and
//     select it (RED / BLUE) instead of entering the naming keyboard.
//  3. Check the controllable-overworld predicate before every input and return
//     as soon as it holds. The real overworld is reached around frame 3310.
//
// It returns the decoded game state at the overworld, or an error naming the
// last decoded state if the overworld is not reached within budget.
func BootToOverworld(m *emu.Emu) (state.GameState, error) {
	var mem state.Mem

	m.StepFrames(300)

	const budget = 900
	for i := 0; i < budget; i++ {
		state.Snapshot(m, &mem)
		if atControllableOverworld(&mem) {
			return state.Decode(&mem), nil
		}
		m.Tap(bootInput(&mem, i), 3, 7)
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
