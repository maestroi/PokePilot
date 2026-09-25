package profile

import (
	"github.com/maestroi/pokepilot/game"
	reddata "github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

func (*Profile) DecodeCapture(reader game.MemoryReader) game.CaptureState {
	if reader == nil {
		return game.CaptureState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	party := state.DecodeParty(&mem)
	box := state.DecodeBox(&mem)
	dex := state.DecodePokedex(&mem)

	out := game.CaptureState{
		PartySpecies:     make([]uint16, 0, len(party.Mons)),
		ActiveBoxSpecies: make([]uint16, 0, len(box.Mons)),
		OwnedDex:         make([]uint16, 0, len(dex.Owned)),
	}
	for _, mon := range party.Mons {
		out.PartySpecies = append(out.PartySpecies, uint16(mon.Species))
	}
	for _, mon := range box.Mons {
		out.ActiveBoxSpecies = append(out.ActiveBoxSpecies, uint16(mon.Species))
	}
	for _, n := range dex.Owned {
		out.OwnedDex = append(out.OwnedDex, uint16(n))
	}
	return out
}

func (*Profile) CaptureDexNumber(romData []byte, nativeSpecies uint16) (uint16, bool) {
	if nativeSpecies == 0 || nativeSpecies > 0xff {
		return 0, false
	}
	dex, err := rom.InternalSpeciesDexNumber(romData, uint8(nativeSpecies))
	if err != nil {
		return 0, false
	}
	return uint16(dex), true
}

func (*Profile) OrdinaryCaptureBallOrder() []uint16 {
	raw := reddata.WildCaptureBallOrder()
	out := make([]uint16, len(raw))
	for i, item := range raw {
		out[i] = uint16(item)
	}
	return out
}
