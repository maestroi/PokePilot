package rom

import "fmt"

// Pokémon Red stores TM/HM policy in three ROM tables:
//   - TechnicalMachines maps machine number (TM01..TM50, HM01..HM05) to move id.
//   - PokedexOrder maps the game's internal species index to Pokédex number.
//   - BaseStats stores seven TM/HM compatibility bytes for each Pokédex entry.
//
// These offsets are the canonical US Red layout already assumed by the other
// parsers in this package (for example Moves in move.go). Keeping the policy in
// the ROM rather than a Go compatibility chart is important for patched ROMs:
// a randomized/edited species should be taught according to the ROM it is
// actually running.
const (
	technicalMachinesOffset = 0x13773 // 04:7773, 55 move ids
	pokedexOrderOffset       = 0x41024 // 10:5024, internal species -> dex number
	pokedexOrderLen          = 190
	baseStatsOffset          = 0x383DE // 0E:43DE, indexed by dex number - 1
	baseStatsEntryLen        = 28
	baseStatsTMHMOffset      = 20
	tmhmBytesPerSpecies      = 7

	HM01Item uint8 = 0xC4
	HM05Item uint8 = 0xC8
	TM01Item uint8 = 0xC9
	TM50Item uint8 = 0xFA

	NumTMs  = 50
	NumHMs  = 5
	NumTMHM = NumTMs + NumHMs
)

// Machine is one TM/HM as represented by the loaded ROM.
// Number is the compatibility-bit number: TM01..TM50 are 1..50 and
// HM01..HM05 are 51..55.
type Machine struct {
	Number int
	Item   uint8
	Move   uint8
	HM     bool
}

// Consumable reports Gen 1 inventory semantics: TMs are removed after a
// successful teach, while HMs are reusable.
func (m Machine) Consumable() bool { return !m.HM }

// MachineNumber converts a bag item id to the ROM's 1-based TM/HM number.
func MachineNumber(item uint8) (number int, hm bool, err error) {
	switch {
	case item >= TM01Item && item <= TM50Item:
		return int(item-TM01Item) + 1, false, nil
	case item >= HM01Item && item <= HM05Item:
		return NumTMs + int(item-HM01Item) + 1, true, nil
	default:
		return 0, false, fmt.Errorf("rom: item %#02x is not a TM or HM", item)
	}
}

// LookupTMHM derives a machine's move from TechnicalMachines in the ROM.
func LookupTMHM(romData []byte, item uint8) (Machine, error) {
	number, hm, err := MachineNumber(item)
	if err != nil {
		return Machine{}, err
	}
	off := technicalMachinesOffset + number - 1
	if off < 0 || off >= len(romData) {
		return Machine{}, fmt.Errorf("rom: TM/HM table entry %d at offset %#x exceeds ROM of %d bytes", number, off, len(romData))
	}
	move := romData[off]
	if move == 0 {
		return Machine{}, fmt.Errorf("rom: TM/HM table entry %d is empty", number)
	}
	if _, err := LookupMove(romData, move); err != nil {
		return Machine{}, fmt.Errorf("rom: TM/HM table entry %d has invalid move %d: %w", number, move, err)
	}
	return Machine{Number: number, Item: item, Move: move, HM: hm}, nil
}

// InternalSpeciesDexNumber converts Red's internal species index to its
// Pokédex number using PokedexOrder. MissingNo slots map to zero and are
// rejected because BaseStats has no legitimate species entry for them.
func InternalSpeciesDexNumber(romData []byte, species uint8) (uint8, error) {
	if species == 0 || int(species) > pokedexOrderLen {
		return 0, fmt.Errorf("rom: internal species %#02x is outside 1..%d", species, pokedexOrderLen)
	}
	off := pokedexOrderOffset + int(species) - 1
	if off >= len(romData) {
		return 0, fmt.Errorf("rom: PokedexOrder entry for species %#02x at offset %#x exceeds ROM of %d bytes", species, off, len(romData))
	}
	dex := romData[off]
	if dex == 0 || dex > 151 {
		return 0, fmt.Errorf("rom: internal species %#02x maps to invalid Pokédex number %d", species, dex)
	}
	return dex, nil
}

// CanLearnTMHM reports the compatibility bit the ROM uses for CanLearnTM.
// It does not infer compatibility from types, learnsets, or species names.
func CanLearnTMHM(romData []byte, species uint8, item uint8) (bool, error) {
	machine, err := LookupTMHM(romData, item)
	if err != nil {
		return false, err
	}
	dex, err := InternalSpeciesDexNumber(romData, species)
	if err != nil {
		return false, err
	}
	entry := baseStatsOffset + (int(dex)-1)*baseStatsEntryLen
	flag := machine.Number - 1
	off := entry + baseStatsTMHMOffset + flag/8
	if off < entry+baseStatsTMHMOffset || off >= entry+baseStatsTMHMOffset+tmhmBytesPerSpecies || off >= len(romData) {
		return false, fmt.Errorf("rom: TM/HM compatibility for dex %d machine %d at offset %#x exceeds ROM of %d bytes", dex, machine.Number, off, len(romData))
	}
	return romData[off]&(1<<uint(flag%8)) != 0, nil
}

// IsHMMove reports whether move is one of the five moves in the ROM's HM
// tail. This deliberately follows TechnicalMachines instead of a canonical
// CUT/FLY/SURF/STRENGTH/FLASH list so patched HM tables stay authoritative.
func IsHMMove(romData []byte, move uint8) (bool, error) {
	if move == 0 {
		return false, nil
	}
	start := technicalMachinesOffset + NumTMs
	end := start + NumHMs
	if end > len(romData) {
		return false, fmt.Errorf("rom: HM table at %#x..%#x exceeds ROM of %d bytes", start, end, len(romData))
	}
	for _, id := range romData[start:end] {
		if id == move {
			return true, nil
		}
	}
	return false, nil
}
