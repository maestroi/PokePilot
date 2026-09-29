package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gsCutTreeCollision       uint8 = 0x12
	gsUnusedCutTreeCollision uint8 = 0x1a
	gsStrengthActiveBit            = 1 << 0
	gsFlashActiveBit               = 1 << 2
	gsPlayerSurf             uint8 = 4
	gsPlayerSurfPikachu      uint8 = 8
	gsTextStateBit                 = 1 << 6
)

// DecodeFieldAction projects the live Gen-II field state used by the shared
// field-action executor. The raw collision ids and player-state values stay on
// the Gold/Silver side of the profile boundary.
func (p *Profile) DecodeFieldAction(reader game.MemoryReader) game.FieldActionState {
	if reader == nil {
		return game.FieldActionState{}
	}
	facing := reader.Peek8(sym.FacingTileID)
	playerState := reader.Peek8(sym.PlayerState)
	_, choice := p.DecodeTwoOption(reader)
	return game.FieldActionState{
		Controllable:     gsControllable(reader),
		CuttableAhead:    facing == gsCutTreeCollision || facing == gsUnusedCutTreeCollision,
		// Strength boulders are object events rather than collision ids. Leave
		// this false until the object-event decoder exposes a semantic target.
		BoulderAhead:     false,
		Surfing:          playerState == gsPlayerSurf || playerState == gsPlayerSurfPikachu,
		StrengthActive:   reader.Peek8(sym.BikeFlags)&gsStrengthActiveBit != 0,
		Lit:              reader.Peek8(sym.StatusFlags)&gsFlashActiveBit != 0,
		ActionSucceeded:  reader.Peek8(sym.FieldMoveSucceeded)&0x0f == 1,
		ResultTextActive: reader.Peek8(sym.StateFlags)&gsTextStateBit != 0,
		ChoiceVisible:    choice,
		DebugText:        gsScreenText(reader),
	}
}

var _ game.FieldActionDecoder = (*Profile)(nil)
