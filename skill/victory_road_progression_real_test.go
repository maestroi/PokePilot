package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestVictoryRoadProgressionRealROM is the focused private qualification for
// #37. POKEPILOT_VICTORY_ROAD_PROGRESSION_STATE should point at a controllable
// eight-badge checkpoint before the final Route 22 rival. The commercial ROM
// and state remain external, following the repository's prepared-state policy.
func TestVictoryRoadProgressionRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_VICTORY_ROAD_PROGRESSION_STATE")
	var before state.Mem
	state.Snapshot(m, &before)
	if got := state.DecodeProgress(&before).BadgeCount; got != 8 {
		t.Fatalf("qualification checkpoint has %d badges, want 8", got)
	}
	if before.U8(sym.CurMap) == indigoPlateauLobbyMap {
		t.Fatal("qualification checkpoint already starts in the Indigo Plateau lobby")
	}

	if err := VictoryRoadProgression(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("VictoryRoadProgression: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if got := after.U8(sym.CurMap); got != indigoPlateauLobbyMap {
		t.Fatalf("final map = %#02x, want Indigo Plateau lobby %#02x", got, indigoPlateauLobbyMap)
	}
	facts := state.DecodeStoryFacts(&after, state.DecodeInventory(&after))
	if !facts.Route22RivalResolved {
		t.Fatal("final Route 22 rival event is not resolved")
	}
	if !facts.Route23BadgeChecksComplete || facts.Route23BadgeChecksPassed != 7 {
		t.Fatalf("Route 23 badge checks = %d/7 complete=%v", facts.Route23BadgeChecksPassed, facts.Route23BadgeChecksComplete)
	}
	if !allPartyCenterRecovered(&after) {
		t.Fatal("Indigo Plateau checkpoint is not fully healed/status-clear/PP-ready")
	}
}

// TestVictoryRoadClearCaveFromExitSidePocketRealROM is the reproduction for
// run-3dtp99mx0jn3ickoqlj1k6iue. POKEPILOT_VICTORY_ROAD_EXIT_POCKET_STATE
// should point at a controllable eight-badge checkpoint standing on 2F's
// 3F-ladder pocket at (25,14): the pocket whose only exit is that ladder, with
// the west boulders walled off behind the east-switch door.
//
// Before the fix clearVictoryRoad demanded the west switch anyway, the push
// search correctly answered "no solution after 1 states", and the objective
// failed forever from a checkpoint the wall kept resuming. The stage must
// instead recognize the exit side and walk out to the Indigo checkpoint, which
// is the clear boundary's own definition of a cleared cave. The same state also
// proves the generic route planner hands the strength executor the ladder pad
// the player is standing on rather than an unreachable same-pair warp.
func TestVictoryRoadClearCaveFromExitSidePocketRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_VICTORY_ROAD_EXIT_POCKET_STATE")
	var before state.Mem
	state.Snapshot(m, &before)
	if got := before.U8(sym.CurMap); got != victoryRoad2FMap {
		t.Fatalf("checkpoint map = %#02x, want Victory Road 2F %#02x", got, victoryRoad2FMap)
	}
	exit, err := victoryRoadExitEdge(m.ROM(), victoryRoad2FMap)
	if err != nil {
		t.Fatalf("2F exit edge: %v", err)
	}
	if warpEdgeReachable(m, m.ROM(), exit) {
		t.Fatal("checkpoint already reaches the 2F exit: it is not the exit-side pocket shape")
	}
	if _, ladderOK, ladderErr := victoryRoadLadderTo3F(m, m.ROM()); ladderErr != nil || !ladderOK {
		t.Fatalf("checkpoint has no reachable 2F->3F ladder (err=%v ok=%v): it is not the exit-side pocket shape", ladderErr, ladderOK)
	}

	if err := VictoryRoadClearCave(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("VictoryRoadClearCave from the exit-side pocket: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if got := after.U8(sym.CurMap); got != indigoPlateauLobbyMap {
		t.Fatalf("final map = %#02x, want Indigo Plateau lobby %#02x", got, indigoPlateauLobbyMap)
	}
	facts := state.DecodeStoryFacts(&after, state.DecodeInventory(&after))
	if !victoryRoadClearBoundaryReady(&after, facts) {
		t.Fatal("final state is not a controllable cleared-cave boundary")
	}
}
