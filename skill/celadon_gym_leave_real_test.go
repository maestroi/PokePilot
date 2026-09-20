package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestCeladonGymLeaveCutPocketRealROM proves that a post-Rainbow resume inside
// Erika's Cut-sealed chamber can still leave through the south door. After
// #1327 moved Cut into destination-aware local pathing, land-only Traverse
// approaches treated the gym trees as solid and reported leg_unwalkable even
// with Cut usable. Warp approaches must use the same field planner (#1339).
//
// The checkpoint must be a controllable CGB state on CELADON_GYM with the
// Rainbow Badge and usable Cut. Farm states for this regression include
// run-yj2tln2booq72zcohb2z6cqmq, run-bhtxa1lziy0w2rj8up54tfc9t, and the
// resumed go_to celadon city terminal
// run-12dm6z8w9rg6332zcpn9ppvdnx (triage fa02e7c6c4a2b3c9 / farm issue 1347).
func TestCeladonGymLeaveCutPocketRealROM(t *testing.T) {
	m := loadPreparedCGBState(t, "POKEPILOT_CELADON_GYM_LEAVE_STATE")
	rom := m.ROM()

	var before state.Mem
	state.Snapshot(m, &before)
	if before.U8(sym.CurMap) != celadonGymMap {
		t.Fatalf("prepared state is on map %#02x, want Celadon Gym %#02x", before.U8(sym.CurMap), celadonGymMap)
	}
	if !state.DecodeProgress(&before).Has(state.BadgeRainbow) {
		t.Fatal("prepared Celadon Gym leave state lacks the Rainbow Badge")
	}
	if !redRouteCapabilities(rom, &before).Has(capCanCut) {
		t.Fatal("prepared Celadon Gym leave state cannot Cut")
	}
	if !state.Controllable(&before) {
		t.Fatal("prepared Celadon Gym leave state is not controllable")
	}

	dest, ok := Place("celadon city")
	if !ok {
		t.Fatal(`Place("celadon city") missing`)
	}
	// Match the terminal farm objective on run-12dm6z8w9rg6332zcpn9ppvdnx
	// ("go to celadon city", not the fleeing variant): Travel must clear the
	// Cut pocket via Traverse's field-path warp approach and land in the city.
	if _, err := Travel(m, rom, dest, StatAwareMove(rom), 20); err != nil {
		t.Fatalf("leave Celadon Gym Cut pocket: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != celadonCityMap {
		t.Fatalf("after leave map=%#02x, want Celadon City %#02x", got, celadonCityMap)
	}
}
