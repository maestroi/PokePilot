package data

import (
	"sort"

	"github.com/maestroi/pokepilot/game"
)

// SpeciesIDs returns every supported non-glitch Red species in a stable order.
// The order is by raw Gen I species index rather than map iteration order, so
// deterministic experiment selection stays reproducible across processes.
func SpeciesIDs() []game.SpeciesID {
	raws := make([]int, 0, len(speciesByID))
	for raw := range speciesByID {
		raws = append(raws, int(raw))
	}
	sort.Ints(raws)
	out := make([]game.SpeciesID, 0, len(raws))
	for _, raw := range raws {
		out = append(out, game.SpeciesID(speciesByID[uint8(raw)]))
	}
	return out
}
