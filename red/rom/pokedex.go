package rom

import "fmt"

// DexNumberInternalSpecies converts a National Pokédex number to Red's sparse
// internal species index by scanning the ROM's PokedexOrder table. This is the
// inverse of InternalSpeciesDexNumber and deliberately follows the loaded ROM
// rather than a hand-maintained Gen 1 species-order table.
func DexNumberInternalSpecies(romData []byte, dex uint8) (uint8, error) {
	if dex == 0 || dex > 151 {
		return 0, fmt.Errorf("rom: Pokédex number %d is outside 1..151", dex)
	}
	end := pokedexOrderOffset + pokedexOrderLen
	if end > len(romData) {
		return 0, fmt.Errorf("rom: PokedexOrder table at %#x..%#x exceeds ROM of %d bytes", pokedexOrderOffset, end, len(romData))
	}

	var found uint8
	for i, mappedDex := range romData[pokedexOrderOffset:end] {
		if mappedDex != dex {
			continue
		}
		internal := uint8(i + 1)
		if found != 0 {
			return 0, fmt.Errorf("rom: Pokédex number %d maps to multiple internal species (%#02x and %#02x)", dex, found, internal)
		}
		found = internal
	}
	if found == 0 {
		return 0, fmt.Errorf("rom: Pokédex number %d has no internal species in PokedexOrder", dex)
	}
	return found, nil
}
