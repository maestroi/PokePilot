package agent

import (
	"strings"
	"testing"
)

// A script press budget must measure stagnation, not total presses. The
// mandatory PlayersHouse1F MeetMom scene needs far more A presses than any
// fixed cap allowed (a long paragraph being drawn, an applymovement owning the
// machine, the clock/day-of-week and YES/NO prompts), yet it is making progress
// the whole time. Recording run-nk4u8m5acmn5 attempt 57 aborted there with
// "exceeded 160 owned A presses" while the text was still advancing.
func TestGSScriptProgressCountsFrozenPagesNotPresses(t *testing.T) {
	var p gsScriptProgress

	// Every press reveals more text: never a stall, no matter how many.
	for i := 0; i < 20*gsScriptFrozenPresses; i++ {
		page := strings.Repeat("x", i)
		if p.observe(page) {
			t.Fatalf("press %d reported a stall while the page was still changing", i)
		}
	}
	if p.presses < 20*gsScriptFrozenPresses {
		t.Fatalf("presses = %d, want the whole run counted", p.presses)
	}
}

func TestGSScriptProgressReportsFrozenPage(t *testing.T) {
	var p gsScriptProgress

	if p.observe("Mom: Here you go!") {
		t.Fatal("first observation reported a stall")
	}
	for i := 1; i < gsScriptFrozenPresses; i++ {
		if p.observe("Mom: Here you go!") {
			t.Fatalf("press %d reported a stall before the cap of %d", i, gsScriptFrozenPresses)
		}
	}
	if !p.observe("Mom: Here you go!") {
		t.Fatalf("press %d did not report a stall after %d unchanged pages",
			gsScriptFrozenPresses, gsScriptFrozenPresses)
	}
}

func TestGSScriptProgressResetClearsFrozenPage(t *testing.T) {
	var p gsScriptProgress

	for i := 0; i < gsScriptFrozenPresses-1; i++ {
		p.observe("Is it Daylight Saving Time now? YES NO")
	}
	p.reset()
	if p.observe("Is it Daylight Saving Time now? YES NO") {
		t.Fatal("reset did not clear the frozen page")
	}
	if p.presses != 1 {
		t.Fatalf("presses = %d, want the counter cleared by reset", p.presses)
	}
}
