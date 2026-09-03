package agent

import "testing"

func TestPokemonTowerObjectiveStringAndItemVocabulary(t *testing.T) {
	o := Objective{Kind: KindPokemonTower}
	if got, want := o.String(), "clear Pokemon Tower and get the POKE FLUTE"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if err := o.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	id, ok := ItemByName("poke flute")
	if !ok || id != 0x49 {
		t.Fatalf("ItemByName(poke flute) = %#02x, %v; want 0x49, true", id, ok)
	}
	name, ok := ItemName(0x49)
	if !ok || name != "poke flute" {
		t.Fatalf("ItemName(0x49) = %q, %v; want poke flute, true", name, ok)
	}
}

func TestOfferPokemonTowerRequiresScopeAndStopsAfterFlute(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{})
	availableMaps := []uint8{
		0x06,                   // Celadon City
		0x85,                   // Celadon Pokemon Center
		0x87,                   // Game Corner
		0xC7, 0xC8, 0xC9, 0xCA, // Rocket Hideout B1F-B4F
		0x12, 0x4D, 0x79, 0x50, 0x13, // Celadon -> Lavender transit
		0x04, 0x8D, // Lavender Town + Center
		0x8E, 0x8F, 0x90, 0x91, 0x92, 0x93, 0x94, // Tower 1F-7F
		0x95, // Mr. Fuji's house
	}

	for _, mapID := range availableMaps {
		obs := Observation{
			Map:        mapID,
			PartyCount: 1,
			Party:      []PartyMon{{Level: 30, HP: 80, MaxHP: 80}},
			Bag:        []Item{{Name: "silph scope", Quantity: 1}},
		}
		if got := countKind(Offer(obs, known), KindPokemonTower); got != 1 {
			t.Errorf("map %#04x offers Pokemon Tower %d times with Scope, want 1", mapID, got)
		}

		obs.Bag = nil
		if got := countKind(Offer(obs, known), KindPokemonTower); got != 0 {
			t.Errorf("map %#04x offers Pokemon Tower %d times without Scope, want 0", mapID, got)
		}

		obs.Bag = []Item{{Name: "silph scope", Quantity: 1}, {Name: "poke flute", Quantity: 1}}
		if got := countKind(Offer(obs, known), KindPokemonTower); got != 0 {
			t.Errorf("map %#04x offers Pokemon Tower %d times after Flute, want 0", mapID, got)
		}
	}

	outside := Observation{
		Map:        0x00,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 30, HP: 80, MaxHP: 80}},
		Bag:        []Item{{Name: "silph scope", Quantity: 1}},
	}
	if got := countKind(Offer(outside, known), KindPokemonTower); got != 0 {
		t.Fatalf("Pokemon Tower offered outside progression slice %d times", got)
	}
}

func countKind(offered []Objective, kind Kind) int {
	count := 0
	for _, o := range offered {
		if o.Kind == kind {
			count++
		}
	}
	return count
}
