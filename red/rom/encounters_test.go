package rom

import (
	"testing"
)

func TestWildEncountersReadsGrassAndSkipsZeroWater(t *testing.T) {
	romData := make([]byte, 0xD400)
	base, err := bankedOffset(wildDataPointersBank, wildDataPointersAddr)
	if err != nil {
		t.Fatal(err)
	}
	record := 0xD100
	addr := uint16(0x5100)
	romData[base+int(mapRoute1)*2] = byte(addr)
	romData[base+int(mapRoute1)*2+1] = byte(addr >> 8)
	// Grass rate 25, one Pidgey (0x24) at level 3, then water rate 0.
	romData[record] = 25
	romData[record+1] = 3
	romData[record+2] = 0x24
	romData[record+1+2*wildSlots] = 0

	got, err := WildEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("encounters = %+v, want one grass Pidgey", got)
	}
	if got[0] != (WildEncounter{MapID: mapRoute1, Habitat: HabitatGrass, Species: 0x24, Level: 3}) {
		t.Fatalf("encounter = %+v, want Route 1 grass Pidgey lv3", got[0])
	}
}

func TestWildEncountersReadsWaterWhenGrassRateIsZero(t *testing.T) {
	romData := make([]byte, 0xD400)
	base, err := bankedOffset(wildDataPointersBank, wildDataPointersAddr)
	if err != nil {
		t.Fatal(err)
	}
	record := 0xD100
	addr := uint16(0x5100)
	romData[base+int(mapRoute21)*2] = byte(addr)
	romData[base+int(mapRoute21)*2+1] = byte(addr >> 8)
	romData[record] = 0 // no grass
	romData[record+1] = 5
	romData[record+2] = 5
	romData[record+3] = 0x18 // Tentacool

	got, err := WildEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (WildEncounter{MapID: mapRoute21, Habitat: HabitatWater, Species: 0x18, Level: 5}) {
		t.Fatalf("encounters = %+v, want Route 21 water Tentacool lv5", got)
	}
}

func TestWildEncountersFollowsPatchedPointer(t *testing.T) {
	romData := make([]byte, 0xD400)
	base, err := bankedOffset(wildDataPointersBank, wildDataPointersAddr)
	if err != nil {
		t.Fatal(err)
	}
	record := 0xD200
	addr := uint16(0x5200)
	romData[base] = byte(addr)
	romData[base+1] = byte(addr >> 8)
	romData[record] = 15
	romData[record+1] = 7
	romData[record+2] = 0xA5 // Rattata
	romData[record+1+2*wildSlots] = 0

	got, err := WildEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Species != 0xA5 || got[0].MapID != 0 {
		t.Fatalf("patched encounter = %+v, want Pallet grass Rattata", got)
	}
}

func TestWildEncountersRoute1AndSeaRoutesFromROM(t *testing.T) {
	romData := loadROM(t)
	got, err := WildEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}

	if !hasWild(got, mapRoute1, HabitatGrass, 0x24) {
		t.Fatalf("Route 1 grass is missing Pidgey: %+v", filterWild(got, mapRoute1))
	}
	if hasWild(got, mapRoute1, HabitatWater, 0x18) {
		t.Fatalf("Route 1 unexpectedly has water encounters")
	}
	if !hasWild(got, 0x1E, HabitatWater, 0x18) { // Route 19
		t.Fatalf("Route 19 water is missing Tentacool")
	}
}

func hasWild(enc []WildEncounter, mapID uint8, habitat string, species uint8) bool {
	for _, e := range enc {
		if e.MapID == mapID && e.Habitat == habitat && e.Species == species {
			return true
		}
	}
	return false
}

func filterWild(enc []WildEncounter, mapID uint8) []WildEncounter {
	var out []WildEncounter
	for _, e := range enc {
		if e.MapID == mapID {
			out = append(out, e)
		}
	}
	return out
}
