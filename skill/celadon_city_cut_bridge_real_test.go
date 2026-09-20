package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestCeladonCityCutBridgeToCenterRealROM proves that a run stranded in the
// Celadon gym-yard / south-street Cut pocket can still GoTo the Pokemon Center.
// After #1356 removed the blind Celadon Gym Cut executor, cross-map GoTo died
// on world.ErrNoRoute even though same-map field pathing could open the street
// toward the Center door (run-18zw4xby92x603chema3m8j2cm, catch blocked
// no_route / triage 08f773ea8ca70f4f).
//
// POKEPILOT_CELADON_CITY_CUT_BRIDGE_STATE should be a controllable CGB state on
// CELADON_CITY inside the Cut-sealed pocket with usable Cut — for example the
// round-004 VULPIX trade checkpoint from that farm run.
func TestCeladonCityCutBridgeToCenterRealROM(t *testing.T) {
	m := loadPreparedCGBState(t, "POKEPILOT_CELADON_CITY_CUT_BRIDGE_STATE")
	rom := m.ROM()

	var before state.Mem
	state.Snapshot(m, &before)
	if before.U8(sym.CurMap) != celadonCityMap {
		t.Fatalf("prepared state is on map %#02x, want Celadon City %#02x", before.U8(sym.CurMap), celadonCityMap)
	}
	if !redRouteCapabilities(rom, &before).Has(capCanCut) {
		t.Fatal("prepared Celadon Cut-bridge state cannot Cut")
	}
	if !state.Controllable(&before) {
		t.Fatal("prepared Celadon Cut-bridge state is not controllable")
	}

	dest, ok := Place("celadon pokemon center")
	if !ok {
		t.Fatal(`Place("celadon pokemon center") missing`)
	}
	if err := GoTo(m, rom, dest); err != nil {
		t.Fatalf("GoTo Celadon Pokemon Center from Cut pocket: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != dest.Map {
		t.Fatalf("after GoTo map=%#02x, want Pokemon Center %#02x", got, dest.Map)
	}
}

// TestCeladonCityCutBridgeToGameCornerRealROM pins the silph_scope sibling of
// the Center bridge (triage d66255b02d6cdbab / run-3hksgfzyx8naz3kvcvmuevzjeo).
// RocketHideout Travels to gameCornerStand; without the field-path bridge that
// cross-map GoTo dies on world.ErrNoRoute from the same (12,28) Cut pocket.
//
// POKEPILOT_CELADON_CITY_CUT_BRIDGE_STATE is shared with the Center test: any
// controllable CGB Celadon Cut-pocket state with usable Cut.
func TestCeladonCityCutBridgeToGameCornerRealROM(t *testing.T) {
	m := loadPreparedCGBState(t, "POKEPILOT_CELADON_CITY_CUT_BRIDGE_STATE")
	rom := m.ROM()

	var before state.Mem
	state.Snapshot(m, &before)
	if before.U8(sym.CurMap) != celadonCityMap {
		t.Fatalf("prepared state is on map %#02x, want Celadon City %#02x", before.U8(sym.CurMap), celadonCityMap)
	}
	if !redRouteCapabilities(rom, &before).Has(capCanCut) {
		t.Fatal("prepared Celadon Cut-bridge state cannot Cut")
	}
	if !state.Controllable(&before) {
		t.Fatal("prepared Celadon Cut-bridge state is not controllable")
	}

	if _, err := Travel(m, rom, gameCornerStand, StatAwareMove(rom), 20); err != nil {
		t.Fatalf("Travel Game Corner from Cut pocket: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != gameCornerStand.Map {
		t.Fatalf("after Travel map=%#02x, want Game Corner %#02x", got, gameCornerStand.Map)
	}
}
