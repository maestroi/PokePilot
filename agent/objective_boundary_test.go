package agent

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
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

// TestCloseOpenMenuToOverworldWaitsOutMenuTeardown pins the window in which a
// dismissed menu is still on the tilemap but its cursor glyph is gone.
//
// MEASURED on the real ROM (post_starter, 2026-09-22): CancelInteraction
// returns as soon as its B lands, but the ROM keeps wFontLoaded set and leaves
// the START frame and "POKéMON ITEM ASH SAVE OPTION EXIT" on the tilemap for
// four more frames, with no cursor glyph. DecodeInteraction therefore reports
// InteractionDialogue for a menu that is already closed, and the boundary used
// to return "dialogue remains open" four frames before control returned —
// turning every dismissed leftover menu into a terminal stabilization failure.
// The wait that fixes it presses nothing, so a panel that really is open is
// still reported on the next pass.
func TestCloseOpenMenuToOverworldWaitsOutMenuTeardown(t *testing.T) {
	e := fixture.Load(t, "post_starter")
	t.Cleanup(func() { e.Close() })

	e.Tap(emu.Start, 3, 7)
	e.StepFrames(30)
	var mem state.Mem
	state.Snapshot(e, &mem)
	if !state.MenuUp(&mem) {
		t.Fatal("setup did not leave the START menu open; the test proves nothing")
	}

	// Back the menu out with the game's own B and stop the moment the cursor
	// glyph is gone but the menu's text is still decoded as an interaction.
	// That is the teardown window: the layer is closed, the ROM is still
	// animating, and the player is not yet in control.
	e.Tap(emu.B, 3, 7)
	found := false
	for i := 0; i < 60; i++ {
		state.Snapshot(e, &mem)
		if !state.MenuUp(&mem) &&
			!state.Controllable(&mem) &&
			state.DecodeInteraction(&mem).Kind == state.InteractionDialogue {
			found = true
			break
		}
		e.StepFrame()
	}
	if !found {
		t.Skip("this ROM did not leave a menu-teardown text window; the test proves nothing")
	}

	before := e.FrameCount()
	if err := skill.CloseOpenMenuToOverworld(e); err != nil {
		t.Fatalf("CloseOpenMenuToOverworld returned %v during the menu's own teardown; want it to wait for control", err)
	}
	state.Snapshot(e, &mem)
	if state.MenuUp(&mem) || !state.Controllable(&mem) {
		t.Fatalf("menu cleanup left menu/control dirty: menu=%v controllable=%v", state.MenuUp(&mem), state.Controllable(&mem))
	}
	if got := e.FrameCount(); got <= before {
		t.Fatalf("CloseOpenMenuToOverworld stepped zero frames during the teardown instead of waiting it out")
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
