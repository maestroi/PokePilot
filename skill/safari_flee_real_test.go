package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestFleeSafariRealROM requires a prepared state captured during an active
// Safari battle. The state is external because repository policy forbids
// committing ROM-derived .state files. Safari RUN is unconditional in the
// ROM; this test proves that Flee can reach that distinct menu and return to
// a controllable overworld instead of waiting forever for the normal FIGHT
// marker.
func TestFleeSafariRealROM(t *testing.T) {
	path := os.Getenv("POKEPILOT_SAFARI_BATTLE_TEST_STATE")
	if path == "" {
		t.Skip("POKEPILOT_SAFARI_BATTLE_TEST_STATE not set (real-ROM prepared-state test)")
	}
	m := openEmu(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read POKEPILOT_SAFARI_BATTLE_TEST_STATE=%s: %v", path, err)
	}
	if err := m.LoadState(b); err != nil {
		t.Fatalf("load POKEPILOT_SAFARI_BATTLE_TEST_STATE=%s: %v", path, err)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	if state.DecodeBattle(&before) == nil {
		t.Fatal("prepared Safari state has no battle in progress")
	}

	if err := Flee(m, 1); err != nil {
		t.Fatalf("Flee from Safari battle: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if state.DecodeBattle(&after) != nil {
		t.Fatal("Safari battle still in progress after RUN")
	}
	if !state.Controllable(&after) {
		t.Fatal("player is not controllable after fleeing Safari battle")
	}
}
