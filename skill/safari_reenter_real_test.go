package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// Farm run-s6v9q3t2w5rl: after a leave declined the gate's re-join prompt,
// the gate script parks the player below the join trigger facing down. The
// next session's entry tapped Up once, which only turned the player, and then
// pressed A for the whole story budget ("enter session 2: story transition
// exceeded 12000 frames on map 0x9c at (3,3)"). Entry must actually step onto
// the trigger so the join prompt opens.
func TestSafariReentryAfterDeclineReachesJoinPromptRealROM(t *testing.T) {
	path := os.Getenv("POKEPILOT_SAFARI_GATE_PROMPT_STATE")
	if path == "" {
		t.Skip("POKEPILOT_SAFARI_GATE_PROMPT_STATE not set (real-ROM prepared-state test)")
	}
	m := openEmuCGB(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.LoadState(b); err != nil {
		t.Fatal(err)
	}
	if err := declineSafariRejoinPrompt(m); err != nil {
		t.Fatalf("decline: %v", err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	t.Logf("after decline: map %#02x (%d,%d) controllable=%v", mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), state.Controllable(&mem))

	gate, _ := Place("safari zone gate")
	if _, err := TravelFlee(m, m.ROM(), gate, StatAwareMove(m.ROM()), fuchsiaTravelEngagements); err != nil {
		t.Fatalf("travel to gate: %v", err)
	}
	if err := stepOntoSafariJoinTrigger(m); err != nil {
		t.Fatalf("step onto join trigger: %v", err)
	}
	if err := driveStoryUntil(m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
		return state.DecodeTwoOptionMenu(mm) != nil
	}); err != nil {
		t.Fatalf("join prompt never opened: %v", err)
	}
	state.Snapshot(m, &mem)
	if _, ok := safariGateJoinChoiceIndex(mem.U8(sym.CurMap), state.ScreenText(&mem), true); !ok {
		t.Fatalf("open choice is not the Safari join prompt: %q", state.ScreenText(&mem))
	}
}
