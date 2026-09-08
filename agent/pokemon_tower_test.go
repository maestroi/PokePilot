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
	if !ok || id != ItemID("poke flute") {
		t.Fatalf("ItemByName(poke flute) = %q, %v; want poke flute, true", id, ok)
	}
	raw, ok := redItemID(id)
	if !ok || raw != 0x49 {
		t.Fatalf("redItemID(poke flute) = %#02x, %v; want 0x49, true", raw, ok)
	}
	name, ok := ItemName(0x49)
	if !ok || name != "poke flute" {
		t.Fatalf("ItemName(0x49) = %q, %v; want poke flute, true", name, ok)
	}
}

func TestOfferPokemonTowerRequiresScopeAndStopsAfterFlute(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{})
	availableMaps := []uint8{
		0x06,
		0x85,
		0x87,
		0xC7, 0xC8, 0xC9, 0xCA,
		0x12, 0x4D, 0x79, 0x50, 0x13,
		0x04, 0x8D,
		0x8E, 0x8F, 0x90, 0x91, 0x92, 0x93, 0x94,
		0x95,
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
