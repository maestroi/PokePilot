package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

// DecodePrompt keeps Red/Blue's prompt text and two-option controller details
// behind the profile boundary. Generic execution can decide what to do with a
// semantic nickname choice without matching cartridge text.
func (*Profile) DecodePrompt(reader game.MemoryReader) game.PromptState {
	if reader == nil {
		return game.PromptState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	if state.DecodeTwoOptionMenu(&mem) == nil {
		return game.PromptState{}
	}
	if strings.Contains(state.ScreenText(&mem), "give a nickname") {
		return game.PromptState{Visible: true, Kind: game.PromptNickname}
	}
	return game.PromptState{Visible: true}
}
