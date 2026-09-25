package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeFieldMoveCapability(r game.MemoryReader, rom []byte, id game.FieldMoveID) (game.FieldMoveCapability, bool, error) {
	return p.engine.DecodeFieldMoveCapability(r, rom, id)
}

func (p *Profile) DecodeFieldMoveMenu(r game.MemoryReader) game.FieldMoveMenuState {
	return p.engine.DecodeFieldMoveMenu(r)
}

func (p *Profile) NativeFieldMove(id game.FieldMoveID) (game.NativeFieldMove, bool) {
	return p.engine.NativeFieldMove(id)
}
