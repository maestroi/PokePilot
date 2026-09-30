package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gsCutTreeCollision  byte = 0x12
	gsCutTreeCollision2 byte = 0x1a
)

// SupportsFieldAction keeps the Gen-II runtime fail-closed while field actions
// migrate one by one. Cut is the first executable action; the remaining
// capability projections do not imply their menu/effect runtime is ready.
func (*Profile) SupportsFieldAction(id game.FieldMoveID) bool {
	return id == game.FieldMoveCut
}

// DecodeFieldAction exposes the retail Cut target/result contract without
// leaking Gen-II collision bytes or wFieldMoveSucceeded into shared skill
// code. wFieldMoveSucceeded aliases wBattlePlayerAction in the pinned WRAM.
func (p *Profile) DecodeFieldAction(reader game.MemoryReader) game.FieldActionState {
	if reader == nil {
		return game.FieldActionState{}
	}
	world := p.DecodeOverworld(reader)
	facing := reader.Peek8(sym.FacingTileID)
	_, choice := p.DecodeTwoOption(reader)
	return game.FieldActionState{
		Controllable:     world.Controllable,
		CuttableAhead:    facing == gsCutTreeCollision || facing == gsCutTreeCollision2,
		ActionSucceeded:  reader.Peek8(sym.BattlePlayerAction) == 1,
		ResultTextActive: world.InDialogue && !choice,
		ChoiceVisible:    choice,
		DebugText:        gsScreenText(reader),
	}
}

var (
	_ game.FieldActionDecoder        = (*Profile)(nil)
	_ game.FieldActionSupportDecoder = (*Profile)(nil)
	_ game.FieldMoveProfile          = (*Profile)(nil)
)
