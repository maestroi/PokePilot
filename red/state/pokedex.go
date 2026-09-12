package state

import "github.com/maestroi/pokepilot/red/sym"

// PokedexState is Red's Pokédex bookkeeping projected into deterministic
// Pokédex-number order. Owned is deliberately separate from Seen: Dex-mode
// completion must use Owned, while Seen is useful only as supporting context.
type PokedexState struct {
	Seen  []uint8
	Owned []uint8
}

// DecodePokedex reads Red's two 151-bit Pokédex arrays directly from WRAM.
// Returned values are National Pokédex numbers (1..151), not Red's sparse
// internal species IDs.
func DecodePokedex(m *Mem) PokedexState {
	return PokedexState{
		Seen:  decodePokedexFlags(m, sym.PokedexSeen),
		Owned: decodePokedexFlags(m, sym.PokedexOwned),
	}
}

func decodePokedexFlags(m *Mem, base uint16) []uint8 {
	out := make([]uint8, 0, sym.PokedexCount)
	for dex := 1; dex <= sym.PokedexCount; dex++ {
		bit := dex - 1
		addr := base + uint16(bit/8)
		mask := byte(1 << uint(bit%8))
		if m.U8(addr)&mask != 0 {
			out = append(out, uint8(dex))
		}
	}
	return out
}
