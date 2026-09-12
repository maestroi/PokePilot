package sym

// Pokédex state. Red stores one bit per National Pokédex number, ordered
// from #1 through #151. 151 bits occupy 19 bytes; the final byte's upper bit
// is padding.
const (
	PokedexOwned uint16 = 0xD2F7 // wPokedexOwned
	PokedexSeen  uint16 = 0xD30A // wPokedexSeen
	PokedexBytes        = 19
	PokedexCount        = 151
)
