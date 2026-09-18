package data

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
)

// SpeciesIDs returns every supported non-glitch Gen-I species in stable raw
// internal-index order.
func SpeciesIDs() []game.SpeciesID {
	out := make([]game.SpeciesID, 0, gen1.SpeciesCount())
	for raw := uint16(1); raw <= 0xff; raw++ {
		if species, ok := gen1.Species(uint8(raw)); ok {
			out = append(out, species)
		}
	}
	return out
}
