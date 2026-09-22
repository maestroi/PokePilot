package agent

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestPrepareObjectiveBoundaryClosesLeftoverStartMenu covers the class of
// stall where a failed item/TM action leaves START/ITEM open. Backing out with
// B is semantically reversible and belongs to objective lifecycle cleanup.
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

// The Museum gate is a gameplay transition owned by Travel/TalkAt, not by the
// generic objective boundary. A dirty checkpoint with the YES/NO open must be
// rejected without sending input; otherwise an unrelated next objective can
// silently buy a ticket on behalf of the objective that leaked it.
func TestPrepareObjectiveBoundaryDoesNotAnswerMuseumGate(t *testing.T) {
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
	beforeFrame := e.FrameCount()

	err = prepareObjectiveBoundary(e)
	if err == nil || !strings.Contains(err.Error(), "unanswered choice remains open") {
		t.Fatalf("prepareObjectiveBoundary err = %v, want unanswered-choice rejection", err)
	}
	if got := e.FrameCount(); got != beforeFrame {
		t.Fatalf("objective boundary stepped %d frames while a choice was open; want zero input", got-beforeFrame)
	}
	state.Snapshot(e, &mem)
	if state.DecodeTwoOptionMenu(&mem) == nil {
		t.Fatal("objective boundary consumed the Museum choice; only Travel/TalkAt may answer it")
	}
}

// TestPrepareObjectiveBoundaryWaitsOutTransientInputLockout covers the class
// of failure that ends runs: a skill fails while the ROM is holding input
// (wJoyIgnore) for an ordinary turn/step animation, with no menu, dialogue or
// battle to explain the lockout. The lockout self-clears a few frames later.
//
// MEASURED on the farm (issues #1533/#1534, run-12pnfwosfyeeq2up888b6daoev):
// Fish's Face() tap at Vermilion City (12,23) left the player uncontrollable
// with nothing on screen for 27 frames. Fish's cast guard failed inside that
// window at frame 22, and the boundary check concluded "dirty" without
// advancing a single frame, so a transient that would have cleared 5 frames
// later was classified as a terminal stabilization failure and ended a
// six-badge run.
//
// The boundary contract may only perform semantically reversible cleanup, and
// waiting is not a gameplay choice: a bounded wait for control to return is
// exactly what GoTo already does (skill.waitOutScriptedMovement /
// skill.Cutscene). Declaring the boundary dirty must wait for the same window
// to elapse first.
func TestPrepareObjectiveBoundaryWaitsOutTransientInputLockout(t *testing.T) {
	// Face()'s input: a short directional tap. Pressing toward an impassable
	// tile turns the player in place, and the ROM holds input for the turn.
	// Try each direction from the fixture's starting position; the turn
	// direction depends on which tile the player faces, not on the button.
	var e *emu.Emu
	var mem state.Mem
	for _, btn := range []emu.Button{emu.Up, emu.Down, emu.Left, emu.Right} {
		cand := fixture.Load(t, "post_starter")
		cand.Tap(btn, 3, 7)
		found := false
		for i := 0; i < 150; i++ {
			state.Snapshot(cand, &mem)
			if !state.Controllable(&mem) &&
				state.DecodeDialogue(&mem) == nil &&
				!state.MenuUp(&mem) &&
				state.DecodeBattle(&mem) == nil {
				// Nothing-open is the precondition of the branch under test;
				// a menu/dialogue/battle open takes a different, already-
				// correct branch and must keep stepping zero frames.
				found = true
				break
			}
			cand.StepFrame()
		}
		if found {
			e = cand
			break
		}
		cand.Close()
	}
	if e == nil {
		t.Skip("this ROM/position did not produce an input lockout; the test proves nothing")
	}
	t.Cleanup(func() { e.Close() })

	before := e.FrameCount()
	err := prepareObjectiveBoundary(e)
	if err != nil {
		t.Fatalf("prepareObjectiveBoundary returned %v on a self-clearing input lockout; want it to wait for control to return", err)
	}
	state.Snapshot(e, &mem)
	if !state.Controllable(&mem) {
		t.Fatalf("boundary recovery left the player uncontrollable after reporting success")
	}
	if got := e.FrameCount(); got == before {
		t.Fatalf("prepareObjectiveBoundary stepped zero frames during the lockout instead of waiting it out")
	}
}

func TestObjectiveBoundaryErrorAttributesDirtyFinishToProducingObjective(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "cerulean city"}
	boundary := errors.New("unanswered choice remains open")
	err := objectiveBoundaryError(o, nil, boundary)
	if !errors.Is(err, boundary) {
		t.Fatalf("postcondition error lost boundary identity: %v", err)
	}
	if !strings.Contains(err.Error(), o.String()) || !strings.Contains(err.Error(), "objective postcondition") {
		t.Fatalf("dirty finish not attributed to producing objective %q: %v", o.String(), err)
	}
}

func TestObjectiveBoundaryErrorPreservesPrimaryFailureIdentity(t *testing.T) {
	primary := errors.New("typed primary failure")
	boundary := errors.New("unanswered choice remains open")
	err := objectiveBoundaryError(Objective{Kind: KindGoTo, Place: "cerulean city"}, primary, boundary)
	if !errors.Is(err, primary) {
		t.Fatalf("combined boundary error lost primary typed failure: %v", err)
	}
	if !strings.Contains(err.Error(), "objective left invalid boundary") || !strings.Contains(err.Error(), boundary.Error()) {
		t.Fatalf("combined error lost dirty-boundary evidence: %v", err)
	}
}
