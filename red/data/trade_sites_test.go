package data

import "testing"

func TestNPCTradeSitesMatchScriptedRedRows(t *testing.T) {
	want := map[int]uint8{
		0: 0x56, // Route 11 Gate 2F
		1: 0x30, // Route 2 Trade House
		3: 0xAA, // Cinnabar Lab Fossil Room
		4: 0xC4, // Vermilion Trade House
		5: 0xBF, // Route 18 Gate 2F
		6: 0x3F, // Cerulean Trade House
		7: 0xA8, // Cinnabar Lab Trade Room (Gramps)
		8: 0xA8, // Cinnabar Lab Trade Room (Beauty)
		9: 0x47, // Underground Path Route 5
	}
	got := NPCTradeSites()
	if len(got) != len(want) {
		t.Fatalf("NPCTradeSites len = %d, want %d", len(got), len(want))
	}
	for _, site := range got {
		mapID, ok := want[site.Index]
		if !ok {
			t.Fatalf("unexpected scripted trade row %d: %+v", site.Index, site)
		}
		if site.Map != mapID || site.Place == "" {
			t.Fatalf("trade row %d = %+v, want map %#02x and non-empty place", site.Index, site, mapID)
		}
		delete(want, site.Index)
	}
	if len(want) != 0 {
		t.Fatalf("missing scripted trade rows: %+v", want)
	}
	if _, ok := NPCTradeSiteByIndex(2); ok {
		t.Fatal("unused Butterfree -> Beedrill row 2 must not have an interaction site")
	}
}
