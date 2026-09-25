package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeCenter(r game.MemoryReader) game.CenterState {
	return p.engine.DecodeCenter(r)
}
