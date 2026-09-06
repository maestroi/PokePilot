package state

import "github.com/maestroi/pokepilot/red/sym"

const activeBoxCapacity = 20

// BoxMon is the subset of a boxed Pokémon needed by progression roster
// planning. Compatibility is species-derived from the ROM and existing HM
// retention is visible in the four stored move IDs.
type BoxMon struct {
	Species uint8
	Moves   [4]uint8
}

// BoxState is Bill's currently active box as mirrored in WRAM. Number is the
// zero-based box number (the ROM stores a separate high bookkeeping bit).
type BoxState struct {
	Number uint8
	Count  uint8
	Mons   []BoxMon
}

// DecodeBox reads the active Bill's PC box. Corrupt/mid-transition counts are
// capped at the Gen I box capacity rather than allowing an out-of-range read.
func DecodeBox(m *Mem) BoxState {
	count := int(m.U8(sym.BoxCount))
	if count > activeBoxCapacity {
		count = activeBoxCapacity
	}
	mons := make([]BoxMon, count)
	for i := 0; i < count; i++ {
		base := sym.BoxMon1 + uint16(i)*sym.BoxMonSize
		mons[i].Species = m.U8(base + sym.BoxMonSpecies)
		copy(mons[i].Moves[:], m.Slice(base+sym.BoxMonMoves, 4))
	}
	return BoxState{
		Number: m.U8(sym.CurrentBoxNum) & 0x7f,
		Count:  uint8(count),
		Mons:   mons,
	}
}
