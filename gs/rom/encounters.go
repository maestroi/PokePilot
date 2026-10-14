package rom

import (
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/maestroi/pokepilot/game"
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

// ClockPeriod maps a wild-table time slot onto the portable clock. Water
// records store an empty time and are not a period.
func ClockPeriod(raw string) (game.TimeOfDay, bool) {
	switch raw {
	case TimeMorning:
		return game.TimeMorning, true
	case TimeDay:
		return game.TimeDay, true
	case TimeNight:
		return game.TimeNight, true
	default:
		return "", false
	}
}

// ClockSlots turns cartridge encounter rows into portable windows. An empty
// Times list means the slot is not clock-gated. Unknown species are dropped.
func ClockSlots(enc []WildEncounter) []game.TimedSlot {
	out := make([]game.TimedSlot, 0, len(enc))
	for _, row := range enc {
		if gsdata.IsEgg(row.Species) {
			continue
		}
		species, ok := gsdata.Species(row.Species)
		if !ok || species == "" {
			continue
		}
		slot := game.TimedSlot{Species: species, Level: row.Level}
		if period, ok := ClockPeriod(row.Time); ok {
			slot.Times = []game.TimeOfDay{period}
		}
		out = append(out, slot)
	}
	return out
}

// encounterCache remembers one ROM's wild rows. The key is a content hash, so
// a stable cartridge pays for LocateTables once per process. The hash is
// recomputed on every lookup; that is cheaper than scanning the tables again.
var encounterCache struct {
	mu  sync.Mutex
	key uint64
	ok  bool
	by  map[uint16][]WildEncounter
}

// EncountersOnMap returns every grass and water slot on one map.
func EncountersOnMap(rom []byte, mapID uint16) ([]WildEncounter, error) {
	by, err := cachedEncountersByMap(rom)
	if err != nil {
		return nil, err
	}
	rows := by[mapID]
	if len(rows) == 0 {
		return nil, nil
	}
	return append([]WildEncounter(nil), rows...), nil
}

func cachedEncountersByMap(rom []byte) (map[uint16][]WildEncounter, error) {
	sum := fnv.New64a()
	_, _ = sum.Write(rom)
	key := sum.Sum64()

	encounterCache.mu.Lock()
	if encounterCache.ok && encounterCache.key == key {
		by := encounterCache.by
		encounterCache.mu.Unlock()
		return by, nil
	}
	encounterCache.mu.Unlock()

	all, err := WildEncounters(rom)
	if err != nil {
		return nil, err
	}
	by := make(map[uint16][]WildEncounter)
	for _, row := range all {
		by[row.MapID] = append(by[row.MapID], row)
	}

	encounterCache.mu.Lock()
	encounterCache.key = key
	encounterCache.by = by
	encounterCache.ok = true
	encounterCache.mu.Unlock()
	return by, nil
}
