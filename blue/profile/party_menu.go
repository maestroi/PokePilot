package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodePartyMenu(r game.MemoryReader) game.PartyMenuState {
	return p.engine.DecodePartyMenu(r)
}
