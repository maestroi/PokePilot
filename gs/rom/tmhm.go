package rom

import "fmt"

const (
	gen2PokemonCount       = 251
	gen2BaseDataEntrySize  = 32
	gen2BaseDataTMHMOffset = 24
	gen2MachineCount       = 57
)

// LocateBaseData finds the supported Gold/Silver BaseData table without
// embedding a retail-ROM address. The pinned Gen-II base-data format stores
// one 32-byte entry per species in National Dex order, and BASE_DEX_NO is the
// first byte of every entry. Requiring the complete 1..251 stride makes the
// locator fail closed on an unsupported or malformed ROM.
func LocateBaseData(romData []byte) (int, error) {
	tableLen := gen2PokemonCount * gen2BaseDataEntrySize
	if len(romData) < tableLen {
		return 0, fmt.Errorf("gs rom: ROM is too small for BaseData: %d bytes", len(romData))
	}

	max := len(romData) - tableLen
	found := -1
	for base := 0; base <= max; base++ {
		// Cheap sentinels avoid walking all 251 records for almost every byte
		// in the cartridge.
		if romData[base] != 1 ||
			romData[base+gen2BaseDataEntrySize] != 2 ||
			romData[base+2*gen2BaseDataEntrySize] != 3 ||
			romData[base+(gen2PokemonCount-1)*gen2BaseDataEntrySize] != byte(gen2PokemonCount) {
			continue
		}

		valid := true
		for species := 1; species <= gen2PokemonCount; species++ {
			if romData[base+(species-1)*gen2BaseDataEntrySize] != byte(species) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs rom: ambiguous BaseData table at %#x and %#x", found, base)
		}
		found = base
	}
	if found < 0 {
		return 0, fmt.Errorf("gs rom: BaseData table not found")
	}
	return found, nil
}

// CanLearnTMHMAt reads one TM/HM compatibility bit from a previously located
// Gen-II BaseData table. machineNumber is TM01..TM50 => 1..50 and
// HM01..HM07 => 51..57.
func CanLearnTMHMAt(romData []byte, base int, species uint8, machineNumber int) (bool, error) {
	if species == 0 || int(species) > gen2PokemonCount {
		return false, fmt.Errorf("gs rom: species %#02x is outside 1..%d", species, gen2PokemonCount)
	}
	if machineNumber < 1 || machineNumber > gen2MachineCount {
		return false, fmt.Errorf("gs rom: machine number %d is outside 1..%d", machineNumber, gen2MachineCount)
	}
	tableEnd := base + gen2PokemonCount*gen2BaseDataEntrySize
	if base < 0 || tableEnd > len(romData) {
		return false, fmt.Errorf("gs rom: BaseData offset %#x is outside ROM size %#x", base, len(romData))
	}

	flag := machineNumber - 1
	offset := base +
		(int(species)-1)*gen2BaseDataEntrySize +
		gen2BaseDataTMHMOffset +
		flag/8
	return romData[offset]&(1<<uint(flag%8)) != 0, nil
}

// CanLearnTMHM locates BaseData and reports whether species may learn the
// requested machine according to the ROM's own compatibility bitmap.
func CanLearnTMHM(romData []byte, species uint8, machineNumber int) (bool, error) {
	base, err := LocateBaseData(romData)
	if err != nil {
		return false, err
	}
	return CanLearnTMHMAt(romData, base, species, machineNumber)
}
