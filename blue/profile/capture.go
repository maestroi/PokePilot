package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeInventory(r game.MemoryReader) game.InventoryState {
	return p.engine.DecodeInventory(r)
}

func (p *Profile) DecodeCapture(r game.MemoryReader) game.CaptureState {
	return p.engine.DecodeCapture(r)
}

func (p *Profile) CaptureDexNumber(romData []byte, species uint16) (uint16, bool) {
	return p.engine.CaptureDexNumber(romData, species)
}

func (p *Profile) OrdinaryCaptureBallOrder() []uint16 {
	return p.engine.OrdinaryCaptureBallOrder()
}
