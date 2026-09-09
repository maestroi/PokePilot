package agent

import "testing"

func TestMtMoonOffersOwnedFossilProgression(t *testing.T) {
	const id ProgressID = "mt_moon_fossil_acquired"
	if !redProgressionKnown(id) {
		t.Fatal("fossil progression is not implemented")
	}
	for _, m := range []uint8{0x0f, 0x3b, 0x3c, 0x3d} {
		obs := Observation{Map: m}
		found := false
		for _, o := range redProgressionObjectives(obs) {
			if o.Progress == id {
				found = true
			}
		}
		if !found {
			t.Errorf("no fossil progression offered on map %02x", m)
		}
		obs.Story = ProgressState{{ID: id, Complete: true}}
		for _, o := range redProgressionObjectives(obs) {
			if o.Progress == id {
				t.Error("completed fossil choice offered again")
			}
		}
	}
}
