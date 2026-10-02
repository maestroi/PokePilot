package rom

import (
	"fmt"

	gsdata "github.com/maestroi/pokepilot/gs/data"
)

// Time of day slots stored in grass wild records. Water has no time split.
const (
	TimeMorning = "morn"
	TimeDay     = "day"
	TimeNight   = "nite"
)

const (
	HabitatGrass = "grass"
	HabitatWater = "water"
)

// WildEncounter is one (map, habitat, time, species, level) slot.
type WildEncounter struct {
	MapID   uint16
	Habitat string
	Time    string
	Species uint8
	Level   uint8
}

// WildEncounters walks every located grass and water table.
func WildEncounters(rom []byte) ([]WildEncounter, error) {
	tables, err := LocateTables(rom)
	if err != nil {
		return nil, err
	}
	return wildEncountersAt(rom, tables)
}

func wildEncountersAt(rom []byte, tables Tables) ([]WildEncounter, error) {
	out := make([]WildEncounter, 0, 1024)
	for _, start := range tables.GrassWild {
		enc, err := readWildTable(rom, start, grassWildLen, true)
		if err != nil {
			return nil, err
		}
		out = append(out, enc...)
	}
	for _, start := range tables.WaterWild {
		enc, err := readWildTable(rom, start, waterWildLen, false)
		if err != nil {
			return nil, err
		}
		out = append(out, enc...)
	}
	return out, nil
}

func readWildTable(rom []byte, start, recLen int, grass bool) ([]WildEncounter, error) {
	var out []WildEncounter
	off := start
	for off+recLen <= len(rom) && rom[off] != 0xff {
		if !looksLikeWildRecord(rom, off, recLen, grass) {
			return nil, fmt.Errorf("gs/rom: malformed %s wild record at %#x", wildKind(grass), off)
		}
		mapID := gsdata.NativeMapID(rom[off], rom[off+1])
		if grass {
			times := []string{TimeMorning, TimeDay, TimeNight}
			slot := off + 5
			for t, time := range times {
				_ = t
				for i := 0; i < 7; i++ {
					level := rom[slot]
					species := rom[slot+1]
					out = append(out, WildEncounter{
						MapID: mapID, Habitat: HabitatGrass, Time: time, Species: species, Level: level,
					})
					slot += 2
				}
			}
		} else {
			slot := off + 3
			for i := 0; i < 3; i++ {
				out = append(out, WildEncounter{
					MapID: mapID, Habitat: HabitatWater, Time: "", Species: rom[slot+1], Level: rom[slot],
				})
				slot += 2
			}
		}
		off += recLen
	}
	return out, nil
}
