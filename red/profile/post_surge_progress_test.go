package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestPostSurgeLavenderReachedIncludesForwardCorridor(t *testing.T) {
	for _, mapID := range []uint8{
		0x04, // Lavender Town
		0x13, // Route 8
		0x79, // Underground Path west-east
		0x12, // Route 7
		0x0A, // Saffron City
		0x06, // Celadon City
		0x85, // Celadon Pokemon Center
		0x87, // Game Corner
		0xC7, // Rocket Hideout 1F
	} {
		if !postSurgeLavenderReached(mapID) {
			t.Errorf("map %#04x (%s) did not count as past the Lavender checkpoint", mapID, state.MapName(mapID))
		}
	}
	for _, mapID := range []uint8{0x05, 0x03, 0x14, 0x52} {
		if postSurgeLavenderReached(mapID) {
			t.Errorf("map %#04x (%s) incorrectly counted as past the Lavender checkpoint", mapID, state.MapName(mapID))
		}
	}
}

func TestPostSurgeCeladonAreaIncludesRelevantInteriors(t *testing.T) {
	for _, mapID := range []uint8{0x06, 0x7A, 0x85, 0x86, 0x87, 0x89, 0xC7, 0xCA} {
		if !postSurgeCeladonArea(mapID) {
			t.Errorf("map %#04x (%s) did not count as Celadon area", mapID, state.MapName(mapID))
		}
	}
	for _, mapID := range []uint8{0x04, 0x13, 0x0A} {
		if postSurgeCeladonArea(mapID) {
			t.Errorf("map %#04x (%s) incorrectly counted as Celadon area", mapID, state.MapName(mapID))
		}
	}
}

func TestPartyCenterRecoveredRequiresFullHPStatusAndPP(t *testing.T) {
	healthy := state.PartyState{
		Count: 1,
		Mons: []state.Mon{{
			HP:    30,
			MaxHP: 30,
			Moves: [4]uint8{1, 2},
			PP:    [4]uint8{10, 5},
		}},
	}
	if !partyCenterRecovered(healthy) {
		t.Fatal("fully recovered party was not recognized")
	}

	damaged := healthy
	damaged.Mons = append([]state.Mon(nil), healthy.Mons...)
	damaged.Mons[0].HP = 29
	if partyCenterRecovered(damaged) {
		t.Fatal("damaged party counted as Center-recovered")
	}

	statused := healthy
	statused.Mons = append([]state.Mon(nil), healthy.Mons...)
	statused.Mons[0].Status = 1
	if partyCenterRecovered(statused) {
		t.Fatal("statused party counted as Center-recovered")
	}

	exhausted := healthy
	exhausted.Mons = append([]state.Mon(nil), healthy.Mons...)
	exhausted.Mons[0].PP[1] = 0
	if partyCenterRecovered(exhausted) {
		t.Fatal("party with an exhausted known move counted as Center-recovered")
	}
}
