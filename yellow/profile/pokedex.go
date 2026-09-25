package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

func yellowPokedexSpecies(reader game.MemoryReader, romData []byte, base uint16) []game.SpeciesID {
	if reader == nil || len(romData) == 0 {
		return nil
	}
	out := make([]game.SpeciesID, 0, 32)
	for dex := 1; dex <= 151; dex++ {
		byteIndex := uint16((dex - 1) / 8)
		bit := uint((dex - 1) % 8)
		if reader.Peek8(base+byteIndex)&(1<<bit) == 0 {
			continue
		}
		raw, err := yellowrom.DexNumberInternalSpecies(romData, uint8(dex))
		if err != nil {
			continue
		}
		id, ok := gen1.Species(raw)
		if !ok {
			continue
		}
		out = append(out, id)
	}
	return out
}

func yellowPokedex(reader game.MemoryReader, romData []byte) (owned, seen []game.SpeciesID) {
	return yellowPokedexSpecies(reader, romData, sym.PokedexOwned),
		yellowPokedexSpecies(reader, romData, sym.PokedexSeen)
}
