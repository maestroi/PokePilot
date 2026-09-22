package rom

import "fmt"

const (
	yellowTechnicalMachinesOffset = 0x1232d // 04:632d TechnicalMachines
	yellowBaseStatsTMHMOffset     = 20
	yellowTMHMBytesPerSpecies     = 7

	HM01Item uint8 = 0xC4
	HM05Item uint8 = 0xC8
	TM01Item uint8 = 0xC9
	TM50Item uint8 = 0xFA

	NumTMs  = 50
	NumHMs  = 5
	NumTMHM = NumTMs + NumHMs
)

type Machine struct {
	Number int
	Item   uint8
	Move   uint8
	HM     bool
}

func (m Machine) Consumable() bool { return !m.HM }

func MachineNumber(item uint8) (number int, hm bool, err error) {
	switch {
	case item >= TM01Item && item <= TM50Item:
		return int(item-TM01Item) + 1, false, nil
	case item >= HM01Item && item <= HM05Item:
		return NumTMs + int(item-HM01Item) + 1, true, nil
	default:
		return 0, false, fmt.Errorf("yellow rom: item %#02x is not a TM or HM", item)
	}
}

func LookupTMHM(romData []byte, item uint8) (Machine, error) {
	number, hm, err := MachineNumber(item)
	if err != nil {
		return Machine{}, err
	}
	off := yellowTechnicalMachinesOffset + number - 1
	if off < 0 || off >= len(romData) {
		return Machine{}, fmt.Errorf("yellow rom: TM/HM table entry %d at %#x exceeds ROM", number, off)
	}
	move := romData[off]
	if move == 0 {
		return Machine{}, fmt.Errorf("yellow rom: TM/HM table entry %d is empty", number)
	}
	if _, err := LookupMove(romData, move); err != nil {
		return Machine{}, fmt.Errorf("yellow rom: TM/HM entry %d move %d: %w", number, move, err)
	}
	return Machine{Number: number, Item: item, Move: move, HM: hm}, nil
}

func CanLearnTMHM(romData []byte, species, item uint8) (bool, error) {
	machine, err := LookupTMHM(romData, item)
	if err != nil {
		return false, err
	}
	dex, err := InternalSpeciesDexNumber(romData, species)
	if err != nil {
		return false, err
	}
	entry := yellowBaseStatsOffset + (int(dex)-1)*yellowBaseStatsEntryLen
	flag := machine.Number - 1
	off := entry + yellowBaseStatsTMHMOffset + flag/8
	end := entry + yellowBaseStatsTMHMOffset + yellowTMHMBytesPerSpecies
	if off < entry+yellowBaseStatsTMHMOffset || off >= end || off >= len(romData) {
		return false, fmt.Errorf("yellow rom: TM/HM compatibility dex=%d machine=%d offset=%#x exceeds ROM", dex, machine.Number, off)
	}
	return romData[off]&(1<<uint(flag%8)) != 0, nil
}

func IsHMMove(romData []byte, move uint8) (bool, error) {
	if move == 0 {
		return false, nil
	}
	start := yellowTechnicalMachinesOffset + NumTMs
	end := start + NumHMs
	if end > len(romData) {
		return false, fmt.Errorf("yellow rom: HM table exceeds ROM")
	}
	for _, id := range romData[start:end] {
		if id == move {
			return true, nil
		}
	}
	return false, nil
}
