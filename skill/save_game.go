package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	saveGameBudget      = 1200
	saveGamePressEvery  = 30
	saveGamePromptText  = "SAVE the game?"
	saveGameEraseText   = "erased"
	saveGameDoneText    = "the game!"
	saveGameDoneSubject = "saved"
)

// SaveGame writes the cartridge's own save from the overworld START menu,
// exactly as a player would. It is the only checkpoint the runtime may return
// to (soft reset + CONTINUE): emulator save states are for diagnostics, never
// runtime recovery (docs/ARCHITECTURE.md §8).
//
// Red's SaveMenu asks "Would you like to SAVE the game?" (and, for a
// different trainer's file, whether to erase it), runs SaveGameData, then
// prints "<PLAYER> saved the game!". That text is the positive proof; A is
// only pressed while a prompt is on screen, because a stray A on the
// returned START menu would select SAVE again.
func SaveGame(m *emu.Emu) error {
	decoder, err := menuDecoderFor(m)
	if err != nil {
		return fmt.Errorf("skill: SaveGame: %w", err)
	}
	if err := openStartMenuEntryWithDecoder(m, decoder, game.StartMenuSave); err != nil {
		return fmt.Errorf("skill: SaveGame: %w", err)
	}
	var mem state.Mem
	saved := false
	for spent := 0; spent < saveGameBudget; spent += saveGamePressEvery {
		state.Snapshot(m, &mem)
		text := state.ScreenText(&mem)
		if strings.Contains(text, saveGameDoneSubject) && strings.Contains(text, saveGameDoneText) {
			saved = true
			break
		}
		if strings.Contains(text, saveGamePromptText) || strings.Contains(text, saveGameEraseText) {
			m.Tap(emu.A, 3, 7)
		}
		m.StepFrames(saveGamePressEvery)
	}
	if !saved {
		return fmt.Errorf("skill: SaveGame: no save confirmation within %d frames; screen=%q", saveGameBudget, strings.Join(strings.Fields(state.ScreenText(&mem)), " "))
	}
	if err := closeStartMenuWithDecoder(m, decoder); err != nil {
		return fmt.Errorf("skill: SaveGame: %w", err)
	}
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		if _, err := m.StepUntil(saveGameBudget, func(mm *emu.Emu) bool {
			state.Snapshot(mm, &mem)
			return state.Controllable(&mem)
		}); err != nil {
			return fmt.Errorf("skill: SaveGame: overworld did not return after saving: %w", err)
		}
	}
	return nil
}
