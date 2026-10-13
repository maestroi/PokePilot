package profile

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestTimeOfDayBytesMatchGetTimeOfDay(t *testing.T) {
	for _, tc := range []struct {
		raw  byte
		want game.TimeOfDay
	}{
		{0, game.TimeMorning},
		{1, game.TimeDay},
		{2, game.TimeNight},
		{3, ""}, // DARKNESS_F is a palette, not a clock period
	} {
		var mem fakeMemory
		mem[sym.TimeOfDay] = tc.raw
		obs, err := NewGold().DecodeObservation(&mem, nil)
		if err != nil {
			t.Fatal(err)
		}
		if obs.TimeOfDay != tc.want {
			t.Fatalf("wTimeOfDay=%d -> %q, want %q", tc.raw, obs.TimeOfDay, tc.want)
		}
	}
}

func TestRegionFromLandmarkFollowsRegionCheck(t *testing.T) {
	for _, tc := range []struct {
		landmark uint8
		want     game.RegionID
	}{
		{0x00, ""},               // LANDMARK_SPECIAL
		{0x01, game.RegionJohto}, // New Bark
		{0x2d, game.RegionJohto}, // last Johto landmark
		{0x2e, game.RegionKanto}, // Pallet
		{0x56, game.RegionKanto}, // last before Victory Road
		{0x57, game.RegionJohto}, // Victory Road
		{0x5e, game.RegionJohto}, // S.S. Aqua
	} {
		if got := regionFromLandmark(tc.landmark); got != tc.want {
			t.Fatalf("landmark %#02x region=%q, want %q", tc.landmark, got, tc.want)
		}
	}
}

func TestDecodeObservationPartyFacts(t *testing.T) {
	rom := syntheticGenderROM(map[uint8]byte{
		0x98: 31,  // Chikorita, GENDER_F12_5
		0x10: 127, // Pidgey, GENDER_F50
		0x51: 255, // Magnemite, GENDER_UNKNOWN
	})
	var mem fakeMemory
	mem[sym.PartyCount] = 3
	mem[sym.PartySpecies+0] = 0x98
	mem[sym.PartySpecies+1] = 0x10
	mem[sym.PartySpecies+2] = 0x51

	// Shiny DVs: attack bit 1 set, defense/speed/special all 10.
	// Combined Attack/Speed DV is 0x2A (42). That is male for a 12.5% female
	// ratio and female for a 50% ratio.
	writeParty := func(slot int, species, pokerus, friendship byte) {
		t.Helper()
		base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize
		mem[base+0x00] = species
		mem[base+partyDVOffset] = 0x2a
		mem[base+partyDVOffset+1] = 0xaa
		mem[base+partyHappinessOffset] = friendship
		mem[base+partyPokerusOffset] = pokerus
	}
	writeParty(0, 0x98, 0x03, 70)
	writeParty(1, 0x10, 0x10, 0)
	base := sym.PartyMon1 + 2*sym.PartyMonSize
	mem[base+0x00] = 0x51
	mem[base+partyHappinessOffset] = 90

	obs, err := NewGold().DecodeObservation(&mem, rom)
	if err != nil {
		t.Fatal(err)
	}
	lead := obs.Party[0]
	if lead.Gender != game.GenderMale || !lead.Shiny || lead.Pokerus != game.PokerusInfected || lead.Friendship != 70 {
		t.Fatalf("chikorita facts=%+v", lead)
	}
	pidgey := obs.Party[1]
	if pidgey.Gender != game.GenderFemale || !pidgey.Shiny || pidgey.Pokerus != game.PokerusCured || pidgey.Friendship != 0 {
		t.Fatalf("pidgey facts=%+v", pidgey)
	}
	magnet := obs.Party[2]
	if magnet.Gender != game.Genderless || magnet.Shiny || magnet.Pokerus != "" || magnet.Friendship != 90 {
		t.Fatalf("magnemite facts=%+v", magnet)
	}
}

func TestGoldRegionsMatchRetailLandmarks(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		want   game.RegionID
		backup string
	}{
		{name: "NEW_BARK_TOWN", want: game.RegionJohto},
		{name: "PALLET_TOWN", want: game.RegionKanto},
		{name: "VICTORY_ROAD", want: game.RegionJohto},
		{name: "FAST_SHIP_1F", want: game.RegionJohto},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := mapByName(t, tc.name)
			var mem fakeMemory
			mem[sym.MapGroup] = info.Group
			mem[sym.MapNumber] = info.Number
			obs, err := NewGold().DecodeObservation(&mem, rom)
			if err != nil {
				t.Fatal(err)
			}
			if obs.Region != tc.want {
				t.Fatalf("%s region=%q, want %q", tc.name, obs.Region, tc.want)
			}
		})
	}

	center := mapByName(t, "POKECENTER_2F")
	pallet := mapByName(t, "PALLET_TOWN")
	newBark := mapByName(t, "NEW_BARK_TOWN")
	var mem fakeMemory
	mem[sym.MapGroup] = center.Group
	mem[sym.MapNumber] = center.Number
	mem[sym.BackupMapGroup] = pallet.Group
	mem[sym.BackupMapNumber] = pallet.Number
	obs, err := NewGold().DecodeObservation(&mem, rom)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Region != game.RegionKanto {
		t.Fatalf("Pokécenter 2F via Pallet region=%q, want kanto", obs.Region)
	}
	mem[sym.BackupMapGroup] = newBark.Group
	mem[sym.BackupMapNumber] = newBark.Number
	obs, err = NewGold().DecodeObservation(&mem, rom)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Region != game.RegionJohto {
		t.Fatalf("Pokécenter 2F via New Bark region=%q, want johto", obs.Region)
	}
}

func mapByName(t *testing.T, name string) gsdata.MapInfo {
	t.Helper()
	for _, info := range gsdata.Maps() {
		if info.Name == name {
			return info
		}
	}
	t.Fatalf("map %s not in catalog", name)
	return gsdata.MapInfo{}
}

func syntheticGenderROM(ratios map[uint8]byte) []byte {
	const entry = 32
	rom := make([]byte, 251*entry)
	for species := 1; species <= 251; species++ {
		off := (species - 1) * entry
		rom[off] = byte(species)
		rom[off+13] = 127
	}
	for species, ratio := range ratios {
		rom[(int(species)-1)*entry+13] = ratio
	}
	return rom
}
