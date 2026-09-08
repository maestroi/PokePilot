package skill

import "fmt"

// WildEncounterSlot is one of Red's ten grass encounter slots before species
// deduplication. Keeping each slot's exact level preserves the ROM's encounter
// weighting for deterministic XP-efficiency estimates.
type WildEncounterSlot struct {
	ID    uint8
	Level uint8
}

// WildGrassSlots returns the exact non-empty grass encounter slots for mapID in
// ROM order. A zero grass rate returns an empty slice, matching WildGrass and
// HasGrass. This is factual encounter data only; no training policy lives here.
func WildGrassSlots(romData []byte, mapID uint8) ([]WildEncounterSlot, error) {
	off, err := wildRecord(romData, mapID)
	if err != nil {
		return nil, err
	}
	if romData[off] == 0 {
		return []WildEncounterSlot{}, nil
	}
	if off+1+2*wildSlots > len(romData) {
		return nil, fmt.Errorf("skill: WildGrassSlots: map %#04x: wild data record runs past the ROM", mapID)
	}
	out := make([]WildEncounterSlot, 0, wildSlots)
	for i := 0; i < wildSlots; i++ {
		level, species := romData[off+1+2*i], romData[off+2+2*i]
		if species == 0 {
			continue
		}
		out = append(out, WildEncounterSlot{ID: species, Level: level})
	}
	return out, nil
}
