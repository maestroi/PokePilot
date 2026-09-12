package rom

import "fmt"

// Wild habitat tags match LoadWildData: grass is the first record half,
// water is the surfing half (pokered/engine/overworld/wild_mons.asm).
const (
	HabitatGrass = "grass"
	HabitatWater = "water"
)

const (
	wildDataPointersBank uint8  = 0x03
	wildDataPointersAddr uint16 = 0x4EEB
	wildSlots                   = 10
	wildMapCount                = 0xF8 // NUM_MAPS: ids 0x00..0xF7
)

// WildEncounter is one (map, habitat, species, level) slot from the ROM's
// grass/water tables. Duplicate slots are preserved so callers can count
// encounter weight the same way the game rolls it.
type WildEncounter struct {
	MapID   uint8
	Habitat string
	Species uint8
	Level   uint8
}

// WildEncounters walks WildDataPointers for every Red map id and returns
// every grass and water slot whose encounter rate is non-zero. A zero rate
// occupies only the rate byte (NothingWildMons / Route 1 water), matching
// LoadWildData's skip.
func WildEncounters(romData []byte) ([]WildEncounter, error) {
	base, err := bankedOffset(wildDataPointersBank, wildDataPointersAddr)
	if err != nil {
		return nil, fmt.Errorf("rom: WildDataPointers: %w", err)
	}
	if base+2*wildMapCount > len(romData) {
		return nil, fmt.Errorf("rom: WildDataPointers at %#x exceeds ROM of %d bytes", base, len(romData))
	}

	out := make([]WildEncounter, 0, 256)
	for mapID := 0; mapID < wildMapCount; mapID++ {
		pOff := base + mapID*2
		addr := uint16(romData[pOff]) | uint16(romData[pOff+1])<<8
		if addr == 0 {
			continue
		}
		rec, err := bankedOffset(wildDataPointersBank, addr)
		if err != nil {
			return nil, fmt.Errorf("rom: wild data map %#02x: %w", mapID, err)
		}
		encounters, next, err := readWildHalf(romData, rec, uint8(mapID), HabitatGrass)
		if err != nil {
			return nil, err
		}
		out = append(out, encounters...)
		encounters, _, err = readWildHalf(romData, next, uint8(mapID), HabitatWater)
		if err != nil {
			return nil, err
		}
		out = append(out, encounters...)
	}
	return out, nil
}

func readWildHalf(romData []byte, off int, mapID uint8, habitat string) ([]WildEncounter, int, error) {
	if off >= len(romData) {
		return nil, 0, fmt.Errorf("rom: wild %s rate for map %#02x at %#x exceeds ROM of %d bytes", habitat, mapID, off, len(romData))
	}
	rate := romData[off]
	off++
	if rate == 0 {
		return nil, off, nil
	}
	if off+2*wildSlots > len(romData) {
		return nil, 0, fmt.Errorf("rom: wild %s slots for map %#02x at %#x exceed ROM of %d bytes", habitat, mapID, off, len(romData))
	}
	out := make([]WildEncounter, 0, wildSlots)
	for i := 0; i < wildSlots; i++ {
		level := romData[off]
		species := romData[off+1]
		off += 2
		if species == 0 {
			continue
		}
		out = append(out, WildEncounter{MapID: mapID, Habitat: habitat, Species: species, Level: level})
	}
	return out, off, nil
}
