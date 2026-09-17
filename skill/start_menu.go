package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	startMenuOpenBudget   = 500
	startMenuRetryWindow  = 25
	startMenuSettleBudget = 100
)

// startMenuReady requires positive evidence from the actual tilemap as well as
// the live menu count. wMaxMenuItem/wCurrentMenuItem survive menu teardown and
// can therefore describe an old two-item menu while the overworld is already
// back on screen; FontLoaded has similar lifecycle gaps. SAVE + EXIT are stable
// start-menu labels, so pairing them with the expected item count identifies
// the menu we are actually trying to drive.
func startMenuReady(mem *state.Mem, wantMax int, a wramAddresses) bool {
	text := state.ScreenText(mem)
	return strings.Contains(text, "SAVE") &&
		strings.Contains(text, "EXIT") &&
		a.DecodeMenu(mem).Max == wantMax
}

func startMenuMarkersVisible(mem *state.Mem) bool {
	text := state.ScreenText(mem)
	return strings.Contains(text, "SAVE") && strings.Contains(text, "EXIT")
}

// waitForStartMenu opens the overworld START menu by positive state rather than
// by a fixed press count. A restored checkpoint can look controllable before
// the overworld input loop is polling again, so the first START (or several)
// can be swallowed. A menu can also be captured mid-close while stale menu RAM
// and tilemap labels are still present. This mirrors the measured retry model
// already used by PromoteToLead: settle a possible closing menu, then keep
// pressing START until the expected menu is actually visible or the bounded
// input budget is exhausted.
func waitForStartMenu(m *emu.Emu, wantMax int) error {
	var mem state.Mem

	// Let a closing start menu finish clearing. A genuinely open menu remains
	// visible for this bounded settle and is accepted immediately afterwards.
	for i := 0; i < startMenuSettleBudget; i++ {
		state.Snapshot(m, &mem)
		if !startMenuMarkersVisible(&mem) {
			break
		}
		m.StepFrame()
	}

	attempts := startMenuOpenBudget / startMenuRetryWindow
	for i := 0; i < attempts; i++ {
		state.Snapshot(m, &mem)
		if startMenuReady(&mem, wantMax, ram(m)) {
			return nil
		}
		if mem.U8(ram(m).IsInBattle) != 0 {
			return fmt.Errorf("skill: start menu cannot open during battle")
		}

		m.Tap(emu.Start, 3, 7)
		if _, err := m.StepUntil(startMenuRetryWindow, func(m *emu.Emu) bool {
			state.Snapshot(m, &mem)
			return startMenuReady(&mem, wantMax, ram(m))
		}); err == nil {
			return nil
		}
	}

	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: start menu did not appear after repeated START presses: screen=%q wJoyIgnore=%#04x wFontLoaded=%#04x wCurrentMenuItem=%#04x wMaxMenuItem=%#04x (want max %d)",
		state.ScreenText(&mem), mem.U16BE(ram(m).JoyIgnore), mem.U8(ram(m).FontLoaded),
		mem.U8(ram(m).CurrentMenuItem), mem.U8(ram(m).MaxMenuItem), wantMax)
}
