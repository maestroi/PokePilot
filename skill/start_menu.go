package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const (
	startMenuOpenBudget   = 500
	startMenuRetryWindow  = 25
	startMenuSettleBudget = 100
)

// waitForStartMenu opens the active game's START menu by positive semantic
// state rather than by a fixed press count. Concrete profiles own the menu's
// labels, story-dependent shape, RAM addresses and battle-state encoding.
func waitForStartMenu(m *emu.Emu) error {
	decoder, err := menuDecoderFor(m)
	if err != nil {
		return err
	}
	return waitForStartMenuWithDecoder(m, decoder)
}

func waitForStartMenuWithDecoder(m menuMachine, decoder game.MenuDecoder) error {
	if decoder == nil {
		return fmt.Errorf("skill: start menu: nil menu decoder")
	}

	// Let a closing START menu finish clearing. A genuinely open menu remains
	// visible for this bounded settle and is accepted immediately afterwards.
	for i := 0; i < startMenuSettleBudget; i++ {
		if !decoder.DecodeStartMenu(m).Visible {
			break
		}
		m.StepFrame()
	}

	attempts := startMenuOpenBudget / startMenuRetryWindow
	for i := 0; i < attempts; i++ {
		state := decoder.DecodeStartMenu(m)
		if state.Ready {
			return nil
		}
		if state.InBattle {
			return fmt.Errorf("skill: start menu cannot open during battle")
		}

		m.Tap(emu.Start, 3, 7)
		if waitMenuUntil(m, startMenuRetryWindow, func() bool {
			return decoder.DecodeStartMenu(m).Ready
		}) {
			return nil
		}
	}

	state := decoder.DecodeStartMenu(m)
	return fmt.Errorf(
		"skill: start menu did not appear after repeated START presses: visible=%v ready=%v in_battle=%v cursor=%d max=%d",
		state.Visible, state.Ready, state.InBattle, state.Cursor.Current, state.Cursor.Max,
	)
}
