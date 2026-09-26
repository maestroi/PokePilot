package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeOverworld(r game.MemoryReader) game.OverworldState {
	return p.engine.DecodeOverworld(r)
}

func (p *Profile) DecodeOverworldBlackout(r game.MemoryReader) game.OverworldBlackoutState {
	return p.engine.DecodeOverworldBlackout(r)
}
