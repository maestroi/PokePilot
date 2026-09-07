package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestPrepareObjectiveBoundaryClosesLeftoverStartMenu covers the class of
// stall where a failed item/TM action leaves START/ITEM open and the planner
// has already chosen an overworld objective. The boundary owns no gameplay
// choice here: B only backs out until Red is controllable again.
func TestPrepareObjectiveBoundaryClosesLeftoverStartMenu(t *testing.T) {
	e := fixture.Load(t, "post_starter")
	e.Tap(emu.Start, 3, 7)
	e.StepFrames(30)

	var mem state.Mem
	state.Snapshot(e, &mem)
	if !state.MenuUp(&mem) {
		t.Fatal("setup did not leave the START menu open; the test proves nothing")
	}

	if err := prepareObjectiveBoundary(e); err != nil {
		t.Fatalf("prepareObjectiveBoundary: %v", err)
	}
	state.Snapshot(e, &mem)
	if state.MenuUp(&mem) || !state.Controllable(&mem) {
		t.Fatalf("boundary recovery left menu/control dirty: menu=%v controllable=%v", state.MenuUp(&mem), state.Controllable(&mem))
	}
}
