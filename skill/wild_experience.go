package skill

import "fmt"

// redWildEncounterChances mirrors pokered/data/wild/probabilities.asm. The
// ten ROM slots are not equiprobable: the first two account for almost 40% of
// encounters, while slot 9 appears only 3/256 of the time.
var redWildEncounterChances = [...]uint16{51, 51, 39, 25, 25, 25, 13, 13, 11, 3}

// WildEncounterSlot is one of Red's ten grass encounter slots before species
// deduplication. Chance is the slot's probability mass out of 256. Keeping the
// exact level, ROM order, and probability lets deterministic XP-efficiency
// estimates use the same distribution the game actually rolls.
type WildEncounterSlot struct {
	ID     uint8
	Level  uint8
	Chance uint16
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
		out = append(out, WildEncounterSlot{ID: species, Level: level, Chance: redWildEncounterChances[i]})
	}
	return out, nil
}
