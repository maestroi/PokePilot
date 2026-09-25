package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeFieldItem(r game.MemoryReader) game.FieldItemState {
	return p.engine.DecodeFieldItem(r)
}

func (p *Profile) FieldItemSemantics(item uint16) game.FieldItemSemantics {
	return p.engine.FieldItemSemantics(item)
}

func (p *Profile) PreferredRepels() []uint16 {
	return p.engine.PreferredRepels()
}
