package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gen2FieldMoveSucceeded byte = 1
	gen2StrengthActiveBit       = 1 << 0
	gen2FlashActiveBit          = 1 << 2
	gen2PlayerSurf         byte = 4
	gen2PlayerSurfPika     byte = 8
	gen2DarknessPalset     byte = 0xff
)

// DecodeFieldAction projects Gold/Silver's live field-move effects into the
// generation-neutral runtime contract. Gold/Silver expose a direct
// wFieldMoveSucceeded result byte, player-state surf mode, the Strength bike
// flag, and the map-local Flash status bit.
//
// Cut/boulder facing-target discovery is intentionally marked unknown here.
// The cartridge still performs the authoritative collision/object check when
// the move is selected, and the generic runtime distinguishes unknown from a
// positively decoded invalid target so this profile never has to guess native
// collision identities.
func (p *Profile) DecodeFieldAction(reader game.MemoryReader) game.FieldActionState {
	if reader == nil {
		return game.FieldActionState{}
	}

	choiceVisible := false
	if _, ok := p.DecodeTwoOption(reader); ok {
		choiceVisible = true
	}
	textVisible := gsTextboxVisible(reader) && !choiceVisible
	playerState := reader.Peek8(sym.PlayerState)
	darkArea := reader.Peek8(sym.TimeOfDayPalset) == gen2DarknessPalset
	flashActive := reader.Peek8(sym.StatusFlags)&gen2FlashActiveBit != 0

	return game.FieldActionState{
		Controllable:       gsControllable(reader),
		CutTargetKnown:     false,
		BoulderTargetKnown: false,
		Surfing:            playerState == gen2PlayerSurf || playerState == gen2PlayerSurfPika,
		StrengthActive:     reader.Peek8(sym.BikeFlags)&gen2StrengthActiveBit != 0,
		Lit:                !darkArea || flashActive,
		ActionSucceeded:    reader.Peek8(sym.FieldMoveSucceeded) == gen2FieldMoveSucceeded,
		ResultTextActive:   textVisible,
		ChoiceVisible:      choiceVisible,
		DebugText:          gsScreenText(reader),
	}
}

var _ game.FieldActionDecoder = (*Profile)(nil)
var _ game.FieldMoveProfile = (*Profile)(nil)
