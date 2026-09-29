package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gen2CollisionCutTreeA = 0x12
	gen2CollisionCutTreeB = 0x1a
	gen2TextStateMask     = 1 << 6
)

func (p *Profile) DecodeFieldAction(reader game.MemoryReader) game.FieldActionState {
	if reader == nil {
		return game.FieldActionState{}
	}
	facing := reader.Peek8(sym.FacingTileID)
	_, choice := p.DecodeTwoOption(reader)
	return game.FieldActionState{
		Controllable:     p.DecodeOverworld(reader).Controllable,
		CuttableAhead:    facing == gen2CollisionCutTreeA || facing == gen2CollisionCutTreeB,
		ActionSucceeded:  reader.Peek8(sym.BattlePlayerAction) == 1, // wFieldMoveSucceeded alias
		ResultTextActive: reader.Peek8(sym.StateFlags)&gen2TextStateMask != 0,
		ChoiceVisible:    choice,
		DebugText:        gsScreenText(reader),
	}
}

var _ game.FieldActionDecoder = (*Profile)(nil)
var _ game.FieldMoveProfile = (*Profile)(nil)
