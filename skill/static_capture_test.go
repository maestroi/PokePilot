package skill

import "testing"

func TestStaticCaptureSitesMatchRedObjects(t *testing.T) {
	want := map[uint8]StaticCaptureSite{
		0x84: {Map: 0x1B, X: 26, Y: 10, StandX: 27, StandY: 10, Requirement: "poke_flute"},
		0x4A: {Map: 0xA2, X: 6, Y: 1, StandX: 6, StandY: 2},
		0x4B: {Map: 0x53, X: 4, Y: 9, StandX: 4, StandY: 10},
		0x49: {Map: 0xC2, X: 11, Y: 5, StandX: 11, StandY: 6},
		0x83: {Map: 0xE3, X: 27, Y: 13, StandX: 27, StandY: 14},
	}
	got := StaticCaptureSites()
	if len(got) != len(want) {
		t.Fatalf("static sites = %d, want %d: %+v", len(got), len(want), got)
	}
	for _, site := range got {
		expect, ok := want[site.Species]
		if !ok {
			t.Fatalf("unexpected static species %#02x at %+v", site.Species, site)
		}
		if site.Map != expect.Map || site.X != expect.X || site.Y != expect.Y || site.StandX != expect.StandX || site.StandY != expect.StandY || site.Requirement != expect.Requirement {
			t.Fatalf("site %#02x = %+v, want map/object/stand/requirement %+v", site.Species, site, expect)
		}
		place, ok := Place(site.Place)
		if !ok || place.Map != site.Map || place.X != site.StandX || place.Y != site.StandY {
			t.Fatalf("Place(%q) = %+v, %v; want map=%#02x stand=(%d,%d)", site.Place, place, ok, site.Map, site.StandX, site.StandY)
		}
	}
}
