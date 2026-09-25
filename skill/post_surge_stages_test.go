package skill

import "testing"

func TestPostSurgeStageGeography(t *testing.T) {
	for _, mapID := range []uint8{0x04, 0x13, 0x79, 0x12, 0x0A, 0x06, 0x85, 0x87, 0xC7} {
		if !postSurgePastLavender(mapID) {
			t.Errorf("map %#04x should count as past Lavender", mapID)
		}
	}
	for _, mapID := range []uint8{0x05, 0x03, 0x14, 0x52} {
		if postSurgePastLavender(mapID) {
			t.Errorf("map %#04x should still require the Lavender stage", mapID)
		}
	}
}

func TestPostSurgeCeladonStageArea(t *testing.T) {
	for _, mapID := range []uint8{0x06, 0x7A, 0x85, 0x86, 0x87, 0x89, 0xC7, 0xCA} {
		if !postSurgeCeladonArea(mapID) {
			t.Errorf("map %#04x should count as Celadon area", mapID)
		}
	}
	for _, mapID := range []uint8{0x04, 0x13, 0x0A} {
		if postSurgeCeladonArea(mapID) {
			t.Errorf("map %#04x should not count as Celadon area", mapID)
		}
	}
}


func TestPostSurgeErikaResumeKeepsGymInterior(t *testing.T) {
	// The final badge stage accepts the whole Celadon area, but a resumed
	// attempt already inside Erika's gym must not route back to the city first.
	// The gym is Cut-sealed on both sides, so that backtrack turns preserved
	// progress into another navigation failure.
	if !postSurgeCeladonArea(celadonGymMap) {
		t.Fatal("Celadon Gym must remain a valid resumable Erika-stage map")
	}
	if postSurgeErikaNeedsCityApproach(celadonGymMap) {
		t.Fatal("resumed Celadon Gym state unexpectedly requires a city approach")
	}
	for _, mapID := range []uint8{celadonCityMap, 0x85, 0x87} {
		if !postSurgeErikaNeedsCityApproach(mapID) {
			t.Errorf("Celadon-area map %#04x should approach through the city before Erika", mapID)
		}
	}
}
