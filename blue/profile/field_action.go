package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeFieldAction(r game.MemoryReader) game.FieldActionState {
	return p.engine.DecodeFieldAction(r)
}
