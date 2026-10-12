package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

var _ game.PromptDecoder = (*Profile)(nil)

// DecodePrompt classifies a live YesNoBox. The nickname question is the one
// generic execution must not accept: egg hatches and AskName default it to
// YES, which opens the naming keyboard (pokegold HatchEggs ->
// _BreedAskNicknameText -> YesNoBox -> NamingScreen).
func (p *Profile) DecodePrompt(reader game.MemoryReader) game.PromptState {
	if _, ok := p.DecodeTwoOption(reader); !ok {
		return game.PromptState{}
	}
	// _CaughtAskNicknameText (catches and GivePoke) is long enough that only
	// "the <MON> you received?" is left on screen once YES/NO is drawn.
	text := gsScreenText(reader)
	if strings.Contains(text, "Give a nickname") || strings.Contains(text, " you received?") {
		return game.PromptState{Visible: true, Kind: game.PromptNickname}
	}
	return game.PromptState{Visible: true}
}

// NamingBufferLength is how many characters the open naming keyboard holds.
func (*Profile) NamingBufferLength(reader game.MemoryReader) int {
	if reader == nil {
		return 0
	}
	return int(reader.Peek8(sym.NamingScreenCurNameLen))
}

// NamingKeyboardOpen reports any naming keyboard (Pokemon, rival, box): its
// bottom row draws "lower/UPPER DEL END", which no dialogue or map renders.
func (*Profile) NamingKeyboardOpen(reader game.MemoryReader) bool {
	return reader != nil && strings.Contains(gsScreenText(reader), "DEL END")
}
