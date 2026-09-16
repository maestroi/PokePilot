package data

import "testing"

func TestStaticCaptureSitesOwnRedFacts(t *testing.T) {
	want := map[uint8]struct {
		mapID             uint8
		x, y              uint8
		standX, standY    uint8
		requirement       string
		wakeItem          uint8
		preferMasterBall  bool
	}{
		0x84: {mapID: 0x1B, x: 26, y: 10, standX: 27, standY: 10, requirement: "poke_flute", wakeItem: 0x49},
		0x4A: {mapID: 0xA2, x: 6, y: 1, standX: 6, standY: 2},
		0x4B: {mapID: 0x53, x: 4, y: 9, standX: 4, standY: 10},
		0x49: {mapID: 0xC2, x: 11, y: 5, standX: 11, standY: 6},
		0x83: {mapID: 0xE3, x: 27, y: 13, standX: 27, standY: 14, preferMasterBall: true},
	}

	sites := StaticCaptureSites()
	if len(sites) != len(want) {
		t.Fatalf("StaticCaptureSites() = %d sites, want %d", len(sites), len(want))
	}
	for _, site := range sites {
		expect, ok := want[site.Species]
		if !ok {
			t.Fatalf("unexpected static species %#02x", site.Species)
		}
		if site.Map != expect.mapID || site.X != expect.x || site.Y != expect.y ||
			site.StandX != expect.standX || site.StandY != expect.standY ||
			site.Requirement != expect.requirement || site.WakeItem != expect.wakeItem ||
			site.PreferMasterBall != expect.preferMasterBall {
			t.Fatalf("site %#02x = %+v, want %+v", site.Species, site, expect)
		}
	}

	// The exported slice is a copy, not mutable adapter state.
	sites[0].Map = 0
	again := StaticCaptureSites()
	if again[0].Map == 0 {
		t.Fatal("StaticCaptureSites returned mutable backing storage")
	}
}

func TestStaticCaptureBallOrderPreservesMasterBallExceptPreferredTarget(t *testing.T) {
	mewtwo, ok := StaticCaptureSiteForSpecies(0x83)
	if !ok {
		t.Fatal("Mewtwo static site missing")
	}
	got := StaticCaptureBallOrder(mewtwo)
	if len(got) != 4 || got[0] != staticMasterBall {
		t.Fatalf("Mewtwo ball order = %#v, want Master Ball first", got)
	}

	articuno, ok := StaticCaptureSiteForSpecies(0x4A)
	if !ok {
		t.Fatal("Articuno static site missing")
	}
	got = StaticCaptureBallOrder(articuno)
	if len(got) != 4 || got[0] == staticMasterBall || got[len(got)-1] != staticMasterBall {
		t.Fatalf("ordinary static ball order = %#v, want Master Ball last", got)
	}
}
