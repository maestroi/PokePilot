package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// These helpers are retained for Red's Safari capture controller and its
// focused regressions. Generic Flee no longer consumes Red tile text or native
// cursor bytes; red/profile projects the same menu through BattleEscapeMenuDecoder.
const (
	fleeMainMenuMarker     = "FIGHT"
	fleeMoveMenuMarker     = "TYPE/"
	safariBattleMenuMarker = "THROW ROCK"

	safariBattleMenuLeftX  byte = 0x01
	safariBattleMenuRightX byte = 0x0d
)

type fleeMenuKind uint8

const (
	fleeMenuNone fleeMenuKind = iota
	fleeMenuNormal
	fleeMenuSafari
)

func fleeMenuFromMem(mem *state.Mem) fleeMenuKind {
	text := state.ScreenText(mem)
	switch {
	case strings.Contains(text, fleeMainMenuMarker):
		return fleeMenuNormal
	case strings.Contains(text, safariBattleMenuMarker):
		return fleeMenuSafari
	default:
		return fleeMenuNone
	}
}

func fleeWaitInputFromMem(mem *state.Mem) emu.Button {
	if strings.Contains(state.ScreenText(mem), fleeMoveMenuMarker) {
		return emu.B
	}
	return emu.A
}

func safariRunCursor(mem *state.Mem) bool {
	return fleeMenuFromMem(mem) == fleeMenuSafari &&
		mem.U8(sym.TopMenuItemX) == safariBattleMenuRightX &&
		int(mem.U8(sym.CurrentMenuItem)) == mainMenuMax
}

func safariRunNextInput(mem *state.Mem) (btn emu.Button, done bool) {
	if safariRunCursor(mem) {
		return 0, true
	}
	row := int(mem.U8(sym.CurrentMenuItem))
	x := mem.U8(sym.TopMenuItemX)
	switch {
	case row < mainMenuMax:
		return emu.Down, false
	case row > mainMenuMax:
		return emu.Up, false
	case x < safariBattleMenuRightX:
		return emu.Right, false
	case x > safariBattleMenuRightX:
		return emu.Left, false
	default:
		return 0, false
	}
}

// waitGen1FleeMenu is retained for the Red-only Safari capture controller.
// Generic Flee uses BattleEscapeMenuDecoder instead.
func waitGen1FleeMenu(m *emu.Emu) (fleeMenuKind, error) {
	start := m.FrameCount()
	for {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if kind := fleeMenuFromMem(&mem); kind != fleeMenuNone {
			return kind, nil
		}
		if int(m.FrameCount()-start) > bagMainMenuBudget {
			return fleeMenuNone, fmt.Errorf("skill: Safari battle menu did not open within %d frames", bagMainMenuBudget)
		}
		m.Tap(fleeWaitInputFromMem(&mem), 3, 7)
	}
}
