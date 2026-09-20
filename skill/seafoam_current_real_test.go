package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestSeafoamCurrentArticunoRouteRealROM is the full qualification for #1340.
//
// The external checkpoint must be a controllable state on Seafoam B4F at the
// west stairs (7,11), before the two final B3F->B4F boulder events are set.
// Strength and Surf must either already be usable or be repairable by the
// ordinary field-capability machinery. The state remains external because
// .state files and the commercial ROM are never committed.
//
// From this exact position Red refuses Surf while the current is active.
// TravelFlee must therefore prove that the current is the blocker, run the
// multi-floor two-boulder drop chain, verify each durable event from RAM,
// discard stale topology, return to B4F, and finish at Articuno's stand.
func TestSeafoamCurrentArticunoRouteRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_SEAFOAM_CURRENT_TEST_STATE")
	romData := m.ROM()
	policy := StatAwareMove(romData)

	var before state.Mem
	state.Snapshot(m, &before)
	if got := before.U8(sym.CurMap); got != seafoamB4FMap {
		t.Fatalf("prepared Seafoam state is on map %#02x, want B4F %#02x", got, seafoamB4FMap)
	}
	if gotX, gotY := before.U8(sym.XCoord), before.U8(sym.YCoord); gotX != seafoamB4FBlockedSurfX || gotY != seafoamB4FBlockedSurfY {
		t.Fatalf("prepared Seafoam state is at (%d,%d), want blocked Surf stair (%d,%d)",
			gotX, gotY, seafoamB4FBlockedSurfX, seafoamB4FBlockedSurfY)
	}
	if state.SeafoamCurrentsStopped(&before) {
		t.Fatal("prepared Seafoam state already has the current stopped")
	}
	if !state.Controllable(&before) {
		t.Fatal("prepared Seafoam state is not controllable")
	}

	// StaticCaptureSite for Articuno uses stand (6,2). Stopping here proves
	// generic routing through the current puzzle without starting the battle.
	dest := Destination{Map: seafoamB4FMap, X: 6, Y: 2}
	if _, err := TravelFlee(m, romData, dest, policy, seafoamTravelBattles); err != nil {
		t.Fatalf("Seafoam B4F blocked stair -> Articuno stand: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !state.SeafoamCurrentsStopped(&after) {
		t.Fatal("Travel reached Articuno side without both final Seafoam current events")
	}
	for _, stage := range []state.SeafoamDropStage{
		state.SeafoamDrop1F,
		state.SeafoamDropB1F,
		state.SeafoamDropB2F,
		state.SeafoamDropB3F,
	} {
		if !state.SeafoamStageComplete(&after, stage) {
			t.Fatalf("Seafoam drop stage %d is incomplete after current preparation", stage)
		}
	}
	if got := after.U8(sym.CurMap); got != dest.Map {
		t.Fatalf("after Seafoam qualification map=%#02x, want %#02x", got, dest.Map)
	}
	if gotX, gotY := after.U8(sym.XCoord), after.U8(sym.YCoord); gotX != dest.X || gotY != dest.Y {
		t.Fatalf("after Seafoam qualification position=(%d,%d), want (%d,%d)", gotX, gotY, dest.X, dest.Y)
	}
	if !state.Controllable(&after) {
		t.Fatal("player is not controllable after Seafoam qualification route")
	}
}
