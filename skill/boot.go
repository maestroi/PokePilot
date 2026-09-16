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
//
// wFontLoaded is not part of the predicate: DisplayIntroNameTextBox draws
// with PlaceString + HandleMenuInput and never goes through DisplayTextID, so
// the oak-speech menus sit at FontLoaded=0. Requiring that flag made every
// A-advance select NEW NAME and type AAAAAAA on the keyboard.
func introNameMenu(m *state.Mem) bool {
	if m.U8(sym.MaxMenuItem) != 3 {
		return false
	}
	return strings.Contains(state.ScreenText(m), "NEW NAME")
}

// introPresetNameIndex is ASH on the player menu and GARY on the rival menu
// (NEW NAME, RED/BLUE, ASH/GARY, JACK/JOHN). Those are the anime names.
const introPresetNameIndex = 2

// bootInput chooses the next deterministic input for the fresh-game intro.
// The first four Start taps preserve the existing title/menu skip. Once a
// player/rival name menu appears, steer to ASH / GARY. Any other intro state
// is ordinary dialogue, where A is the safe paging input.
func bootInput(m *state.Mem, iteration int) emu.Button {
	if iteration < 4 {
		return emu.Start
	}
	if !introNameMenu(m) {
		return emu.A
	}

	current := m.U8(sym.CurrentMenuItem)
	switch {
	case current < introPresetNameIndex:
		return emu.Down
	case current > introPresetNameIndex:
		return emu.Up
	default:
		return emu.A
	}
}

// BootToOverworld drives a fresh Pokemon Red from power-on to a controllable
// overworld, verifying the end state from RAM rather than pixels.
//
// The sequence, measured on this ROM:
//  1. StepFrames(300) to let the game boot.
//  2. Loop up to 900 iterations. The first 4 iterations tap Start to clear the
//     title/menu. Ordinary intro dialogue taps A. When Oak's player or rival
//     name menu appears, detect it from RAM/text, move to preset index 2 and
//     select it (ASH / GARY) instead of entering the naming keyboard.
//  3. Check the controllable-overworld predicate before every input and return
//     as soon as it holds. The real overworld is reached around frame 3310.
//  4. Verify the player is ASH and the rival is GARY. Reaching the bedroom
//     named AAAAAAA is not success: that is the naming-keyboard failure mode.
//
// It returns the decoded game state at the overworld, or an error naming the
// last decoded state if the overworld is not reached within budget.
func BootToOverworld(m *emu.Emu) (state.GameState, error) {
	var mem state.Mem

	m.StepFrames(300)

	// expectedNames learns each preset name from the menus as the boot drives
	// them, in the order Oak asks (player, then rival). Decoding the selection
	// back out of RAM proves the boot picked presets rather than typing.
	var expectedNames []string

	const budget = 900
	for i := 0; i < budget; i++ {
		state.Snapshot(m, &mem)
		if atControllableOverworld(&mem) {
			return decodeBootedOverworld(&mem, expectedNames)
		}
		// The cursor rests on the selected preset for several frames while A is
		// pressed, so this is the robust moment to read the tilemap. Deduping
		// against the last entry keeps one capture per menu.
		if introNameMenu(&mem) && mem.U8(sym.CurrentMenuItem) == introPresetNameIndex {
			if names := presetMenuNames(state.ScreenText(&mem)); len(names) >= introPresetNameIndex {
				name := names[introPresetNameIndex-1]
				if n := len(expectedNames); n == 0 || expectedNames[n-1] != name {
					expectedNames = append(expectedNames, name)
				}
			}
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

// presetMenuNames extracts the built-in name entries shown on one of Oak's
// preset-name menus. The tilemap flattens to
//
//	"NAME NEW NAME <preset1> <preset2> <preset3> <prompt sentence>"
//
// so the entries are the uppercase words following NEW NAME. Reading them from
// the game keeps the boot version-agnostic: Red and Blue swap their presets,
// and any other Gen I revision would differ again without a code change here.
func presetMenuNames(screenText string) []string {
	fields := strings.Fields(screenText)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] != "NEW" || fields[i+1] != "NAME" {
			continue
		}
		var names []string
		for _, field := range fields[i+2:] {
			if !isPresetWord(field) {
				break
			}
			names = append(names, field)
		}
		return names
	}
	return nil
}

func isPresetWord(field string) bool {
	if len(field) < 2 {
		return false
	}
	for _, r := range field {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// decodeBootedOverworld verifies the party names against the presets the boot
// selected. A typed name instead (the historical failure mode was "AAAAAAA")
// means the boot answered NEW NAME and fell into the naming keyboard.
func decodeBootedOverworld(mem *state.Mem, expectedNames []string) (state.GameState, error) {
	for i, addr := range []uint16{sym.PlayerName, sym.RivalName} {
		if i >= len(expectedNames) {
			break
		}
		if got := state.DecodeName(mem.Slice(addr, 11)); got != expectedNames[i] {
			return state.GameState{}, fmt.Errorf("boot: reached overworld with name %q, want preset %q", got, expectedNames[i])
		}
	}
	return state.Decode(mem), nil
}
