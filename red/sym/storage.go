package sym

// Active Bill's PC box. Pokémon Red mirrors only the currently selected box
// into WRAM; other boxes live in SRAM until Change Box swaps one into this
// buffer. These addresses are from pokered.sym / ram/wram.asm.
const (
	CurrentBoxNum uint16 = 0xD5A0 // low 7 bits are the active box number
	BoxCount      uint16 = 0xDA80
	BoxSpecies    uint16 = 0xDA81
	BoxMon1       uint16 = 0xDA96
	BoxMonSize    uint16 = 0x21 // BOXMON_STRUCT_LENGTH
)

// Offsets within one boxed-mon struct. Boxed mons do not carry calculated
// battle stats, but species and moves are enough for roster/HM planning.
const (
	BoxMonSpecies uint16 = 0x00
	BoxMonMoves   uint16 = 0x08
)
