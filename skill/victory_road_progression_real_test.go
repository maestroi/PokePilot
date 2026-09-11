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
