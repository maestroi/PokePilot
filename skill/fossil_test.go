package skill

import "testing"

func TestFossilItemForSpecies(t *testing.T) {
	cases := []struct {
		species uint8
		item    uint8
	}{
		{species: kabutoSpecies, item: domeFossilItem},
		{species: omanyteSpecies, item: helixFossilItem},
		{species: aerodactylSpecies, item: oldAmberItem},
	}
	for _, tc := range cases {
		got, ok := fossilItemForSpecies(tc.species)
		if !ok || got != tc.item {
			t.Fatalf("fossilItemForSpecies(%#02x) = (%#02x, %v), want (%#02x, true)", tc.species, got, ok, tc.item)
		}
	}
	if _, ok := fossilItemForSpecies(0x19); ok {
		t.Fatal("non-fossil species unexpectedly resolved to a fossil item")
	}
}

func TestFossilRevivalPlace(t *testing.T) {
	name := FossilRevivalPlace()
	dest, ok := Place(name)
	if !ok {
		t.Fatalf("Place(%q) did not resolve", name)
	}
	if dest.Map != cinnabarFossilRoomMap || dest.X != 2 || dest.Y != 6 {
		t.Fatalf("fossil revival destination = %+v", dest)
	}
}

func TestIsFossilRevivalActor(t *testing.T) {
	if !IsFossilRevivalActor(cinnabarFossilRoomMap, fossilScientistX, fossilScientistY) {
		t.Fatal("fossil scientist was not classified as semantic revival actor")
	}
	if IsFossilRevivalActor(cinnabarFossilRoomMap, 7, 6) {
		t.Fatal("NPC trade scientist was misclassified as fossil revival actor")
	}
}
