package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeShop(r game.MemoryReader) game.ShopState {
	return p.engine.DecodeShop(r)
}
