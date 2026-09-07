package agent

import (
	"os"
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

// TestPrepareObjectiveBoundaryAnswersMuseumGate is the farm death after
// Travel left Museum 1F's ticket YES/NO up: the next objective used to die
// on "unanswered choice remains open" before TalkAt/Travel could pay.
func TestPrepareObjectiveBoundaryAnswersMuseumGate(t *testing.T) {
	path := os.Getenv("REPRO_TALK_STATE")
	if path == "" {
		path = "/tmp/r3ray-r53-talk.state"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("museum gate state not present: %v", err)
	}
	rom := os.Getenv("POKEMON_RED_ROM")
	if rom == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	e, err := emu.Open(rom)
	if err != nil {
		t.Fatalf("emu.Open: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	if err := e.LoadState(b); err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	if state.DecodeTwoOptionMenu(&mem) == nil {
		t.Fatal("setup did not leave the museum YES/NO open; the test proves nothing")
	}

	if err := prepareObjectiveBoundary(e); err != nil {
		t.Fatalf("prepareObjectiveBoundary: %v", err)
	}
	state.Snapshot(e, &mem)
	if state.DecodeTwoOptionMenu(&mem) != nil || !state.Controllable(&mem) {
		t.Fatalf("museum gate still open: choice=%v controllable=%v text=%q",
			state.DecodeTwoOptionMenu(&mem) != nil, state.Controllable(&mem), state.ScreenText(&mem))
	}
}
