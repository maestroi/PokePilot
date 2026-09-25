package rom

import (
	"fmt"

	"github.com/maestroi/pokepilot/gen1rom"
)

// Rod identities follow the three item-effect handlers in
// pokered/engine/items/item_effects.asm.
const (
	RodOld   = "old"
	RodGood  = "good"
	RodSuper = "super"
)

const (
	goodRodEntries     = 2
	oldRodOpcodeOffset = 6 // after `call FishingInit` + `jp c, ...`
	superRodInlineMons = 4 // SuperRodFishingSlots: map + four (species, level)
)

// FishingEncounter is one rod table slot. Old and Good rods are global
// (MapID 0): they are not keyed by map. Super Rod rows carry the map the
// SuperRodData table names.
type FishingEncounter struct {
	MapID   uint8
	Rod     string
	Species uint8
	Level   uint8
}

// FishingEncounters reads Old Rod (the ItemUseOldRod `lb bc, level, species`
// immediate), GoodRodMons, and SuperRodData. Those are the only fishing
// tables the ROM has; there is no hand-maintained Magikarp/Goldeen list.
func FishingEncounters(romData []byte) ([]FishingEncounter, error) {
	out := make([]FishingEncounter, 0, 64)

	old, err := oldRodEncounter(romData)
	if err != nil {
		return nil, err
	}
	out = append(out, old)

	good, err := goodRodEncounters(romData)
	if err != nil {
		return nil, err
	}
	out = append(out, good...)

	super, err := superRodEncounters(romData)
	if err != nil {
		return nil, err
	}
	out = append(out, super...)
	return out, nil
}

func oldRodEncounter(romData []byte) (FishingEncounter, error) {
	off, err := Tables(romData).ItemUseOldRod.Offset()
	if err != nil {
		return FishingEncounter{}, fmt.Errorf("rom: ItemUseOldRod: %w", err)
	}
	// `lb bc, 5, MAGIKARP` encodes as `ld bc, (level<<8)|species`:
	// opcode 0x01, C = species, B = level.
	imm := off + oldRodOpcodeOffset
	if imm+3 > len(romData) {
		return FishingEncounter{}, fmt.Errorf("rom: ItemUseOldRod immediate at %#x exceeds ROM of %d bytes", imm, len(romData))
	}
	if romData[imm] != 0x01 {
		return FishingEncounter{}, fmt.Errorf("rom: ItemUseOldRod at %#x is not ld bc,imm16 (got %#02x)", imm, romData[imm])
	}
	return FishingEncounter{Rod: RodOld, Species: romData[imm+1], Level: romData[imm+2]}, nil
}

func goodRodEncounters(romData []byte) ([]FishingEncounter, error) {
	off, err := Tables(romData).GoodRodMons.Offset()
	if err != nil {
		return nil, fmt.Errorf("rom: GoodRodMons: %w", err)
	}
	if off+2*goodRodEntries > len(romData) {
		return nil, fmt.Errorf("rom: GoodRodMons at %#x exceeds ROM of %d bytes", off, len(romData))
	}
	out := make([]FishingEncounter, 0, goodRodEntries)
	for i := 0; i < goodRodEntries; i++ {
		level := romData[off]
		species := romData[off+1]
		off += 2
		if species == 0 {
			continue
		}
		out = append(out, FishingEncounter{Rod: RodGood, Species: species, Level: level})
	}
	return out, nil
}

func superRodEncounters(romData []byte) ([]FishingEncounter, error) {
	layout := Tables(romData)
	off, err := layout.SuperRod.Offset()
	if err != nil {
		return nil, fmt.Errorf("rom: SuperRodData: %w", err)
	}
	if layout.SuperRodFormat == gen1rom.SuperRodInline {
		return inlineSuperRodEncounters(romData, off)
	}
	out := make([]FishingEncounter, 0, 64)
	for {
		if off >= len(romData) {
			return nil, fmt.Errorf("rom: SuperRodData terminator missing before end of ROM")
		}
		if romData[off] == 0xFF {
			return out, nil
		}
		if off+3 > len(romData) {
			return nil, fmt.Errorf("rom: SuperRodData entry at %#x exceeds ROM of %d bytes", off, len(romData))
		}
		mapID := romData[off]
		addr := uint16(romData[off+1]) | uint16(romData[off+2])<<8
		off += 3
		group, err := bankedOffset(layout.SuperRod.Bank, addr)
		if err != nil {
			return nil, fmt.Errorf("rom: SuperRodData map %#02x: %w", mapID, err)
		}
		if group >= len(romData) {
			return nil, fmt.Errorf("rom: SuperRodData group for map %#02x at %#x exceeds ROM of %d bytes", mapID, group, len(romData))
		}
		count := int(romData[group])
		if group+1+2*count > len(romData) {
			return nil, fmt.Errorf("rom: SuperRodData group for map %#02x runs past ROM", mapID)
		}
		for i := 0; i < count; i++ {
			level := romData[group+1+2*i]
			species := romData[group+2+2*i]
			if species == 0 {
				continue
			}
			out = append(out, FishingEncounter{MapID: mapID, Rod: RodSuper, Species: species, Level: level})
		}
	}
}

// inlineSuperRodEncounters reads Yellow's SuperRodFishingSlots: each row is a
// map id and four inline (species, level) slots, terminated by $ff
// (pokeyellow/engine/items/super_rod.asm).
func inlineSuperRodEncounters(romData []byte, off int) ([]FishingEncounter, error) {
	const rowLen = 1 + 2*superRodInlineMons
	out := make([]FishingEncounter, 0, 128)
	for {
		if off >= len(romData) {
			return nil, fmt.Errorf("rom: SuperRodFishingSlots terminator missing before end of ROM")
		}
		if romData[off] == 0xFF {
			return out, nil
		}
		if off+rowLen > len(romData) {
			return nil, fmt.Errorf("rom: SuperRodFishingSlots row at %#x exceeds ROM of %d bytes", off, len(romData))
		}
		mapID := romData[off]
		for i := 0; i < superRodInlineMons; i++ {
			species := romData[off+1+2*i]
			level := romData[off+2+2*i]
			if species == 0 {
				continue
			}
			out = append(out, FishingEncounter{MapID: mapID, Rod: RodSuper, Species: species, Level: level})
		}
		off += rowLen
	}
}
