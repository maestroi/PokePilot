package rom

import (
	"fmt"

	"github.com/maestroi/pokepilot/gen1rom"
)

const (
	OldRodItem   uint8 = 0x4c
	GoodRodItem  uint8 = 0x4d
	SuperRodItem uint8 = 0x4e

	yellowGoodRodBank uint8  = 0x03
	yellowGoodRodAddr uint16 = 0x612c

	yellowSuperRodBank uint8  = 0x3d
	yellowSuperRodAddr uint16 = 0x5eda
)

type FishingEncounter struct {
	Rod     uint8
	MapID   uint8
	Species uint8
	Level   uint8
	Global  bool
}

// FishingEncounters decodes Yellow's real rod encounter data. Old/Good Rod
// are global once the player is facing a legal shoreline; Super Rod is
// explicitly map-scoped in SuperRodFishingSlots.
func FishingEncounters(romData []byte) ([]FishingEncounter, error) {
	out := []FishingEncounter{
		{Rod: OldRodItem, Species: 0x85, Level: 5, Global: true}, // MAGIKARP
	}

	goodOff, err := gen1rom.BankedOffset(yellowGoodRodBank, yellowGoodRodAddr)
	if err != nil {
		return nil, err
	}
	if goodOff+4 > len(romData) {
		return nil, fmt.Errorf("yellow rom: GoodRodMons at %#x exceeds ROM", goodOff)
	}
	// GoodRodMons stores two (level,species) pairs.
	for i := 0; i < 2; i++ {
		level := romData[goodOff+i*2]
		species := romData[goodOff+i*2+1]
		if species == 0 || level == 0 {
			return nil, fmt.Errorf("yellow rom: invalid Good Rod entry %d level=%d species=%#02x", i, level, species)
		}
		out = append(out, FishingEncounter{
			Rod: GoodRodItem, Species: species, Level: level, Global: true,
		})
	}

	superOff, err := gen1rom.BankedOffset(yellowSuperRodBank, yellowSuperRodAddr)
	if err != nil {
		return nil, err
	}
	// SuperRodFishingSlots entries are map id followed by four
	// (species,level) pairs; $ff terminates the table.
	for entry := 0; ; entry++ {
		at := superOff + entry*9
		if at >= len(romData) {
			return nil, fmt.Errorf("yellow rom: SuperRodFishingSlots has no terminator")
		}
		mapID := romData[at]
		if mapID == 0xff {
			break
		}
		if at+9 > len(romData) {
			return nil, fmt.Errorf("yellow rom: Super Rod entry %d at %#x exceeds ROM", entry, at)
		}
		if MapName(mapID) == "" {
			return nil, fmt.Errorf("yellow rom: Super Rod entry %d has invalid map %#02x", entry, mapID)
		}
		for slot := 0; slot < 4; slot++ {
			species := romData[at+1+slot*2]
			level := romData[at+2+slot*2]
			if species == 0 || level == 0 {
				return nil, fmt.Errorf("yellow rom: invalid Super Rod entry map=%#02x slot=%d", mapID, slot)
			}
			out = append(out, FishingEncounter{
				Rod: SuperRodItem, MapID: mapID, Species: species, Level: level,
			})
		}
	}
	return out, nil
}
