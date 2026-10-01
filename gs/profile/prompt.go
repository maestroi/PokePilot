package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
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
	if strings.Contains(gsScreenText(reader), "Give a nickname") {
		return game.PromptState{Visible: true, Kind: game.PromptNickname}
	}
	return game.PromptState{Visible: true}
}

// NamingKeyboardOpen reports any naming keyboard (Pokemon, rival, box): its
// bottom row draws "lower/UPPER DEL END", which no dialogue or map renders.
func (*Profile) NamingKeyboardOpen(reader game.MemoryReader) bool {
	return reader != nil && strings.Contains(gsScreenText(reader), "DEL END")
}
