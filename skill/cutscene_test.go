package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// TestCutsceneEnduresOakGate is the measurement that confirms S3-1's derived
// bit index 0 for EVENT_FOLLOWED_OAK_INTO_LAB. From a booted game it walks to
// Pallet Town, walks north into the gate (y==1, where wJoyIgnore is set), and
// lets the cutscene play. It asserts the event flag flipped, input is back,
// and the player is controllable.
//
// If the flag never flips but the player does become controllable on a
// different map, the bit index is wrong: the test prints the full
// 0xD747..0xD74F bytes before and after and fails, rather than weakening the
// predicate.
func TestCutsceneEnduresOakGate(t *testing.T) {
	e := loadFixture(t)

	// Booted game: walk to Pallet Town.
	dest, ok := skill.Place("pallet town")
	if !ok {
		t.Fatal(`Place: "pallet town" not found`)
	}
	if err := skill.GoTo(e, e.ROM(), dest); err != nil {
		t.Fatalf("GoTo pallet town: %v", err)
	}

	// Walk north toward the exit. The path is verified walkable on the real
	// map (20x18 tiles): right to a clear column, up to the exit row, right to
	// the exit, up to y==1. The gate fires at y==1.
	path := []world.Step{
		world.StepRight, world.StepRight, world.StepRight, // (5,6) -> (8,6)
		world.StepUp, world.StepUp, world.StepUp, world.StepUp, // (8,6) -> (8,2)
		world.StepRight, world.StepRight, // (8,2) -> (10,2)
		world.StepUp, // (10,2) -> (10,1): the gate fires at y==1
	}
	if err := skill.WalkPath(e, path); err != nil {
		t.Fatalf("WalkPath to the north exit: %v", err)
	}

	// The gate fires a frame or two after the player reaches y==1: wait for
	// wJoyIgnore to become non-zero. If it never does, the gate did not fire.
	if _, err := e.StepUntil(60, func(m *emu.Emu) bool {
		return m.Peek8(sym.JoyIgnore) != 0
	}); err != nil {
		var mem state.Mem
		state.Snapshot(e, &mem)
		t.Fatalf("gate did not fire: wJoyIgnore stayed 0 at map=%#04x (%d,%d)",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}

	// The step into the exit is now blocked: the game is driving, not us.
	var mem state.Mem
	state.Snapshot(e, &mem)
	x, y := mem.U8(sym.XCoord), mem.U8(sym.YCoord)
	joyIgnore := mem.U8(sym.JoyIgnore)
	if y != 1 {
		t.Fatalf("gate fired at y=%d (x=%d), want y=1; wJoyIgnore=%#04x", y, x, joyIgnore)
	}
	if joyIgnore == 0 {
		t.Fatalf("wJoyIgnore = 0 at (%d,%d), want non-zero (the gate)", x, y)
	}
	if err := skill.StepOnce(e, world.StepUp); err == nil {
		t.Fatalf("StepUp into the exit succeeded at (%d,%d); the gate did not block it", x, y)
	}
	t.Logf("gate fired at (%d,%d), wJoyIgnore=%#04x; step into the exit is blocked", x, y, joyIgnore)

	// Capture the event-flag bytes (0xD747..0xD74F) before the cutscene.
	state.Snapshot(e, &mem)
	before := mem.Slice(sym.EventFlags, 11)

	// Let the cutscene play. done = the story flag flipped.
	if err := skill.Cutscene(e, 30000, func(m *state.Mem) bool {
		return state.HasEvent(m, state.EventFollowedOakIntoLab)
	}); err != nil {
		t.Fatalf("Cutscene: %v", err)
	}

	// Assert the positive facts: the flag flipped, input is back, controllable.
	state.Snapshot(e, &mem)
	after := mem.Slice(sym.EventFlags, 11)
	if !state.HasEvent(&mem, state.EventFollowedOakIntoLab) {
		t.Fatalf("EVENT_FOLLOWED_OAK_INTO_LAB did not flip; S3-1 bit index 0 is wrong. "+
			"wEventFlags before=%x after=%x map=%#04x at (%d,%d) controllable=%v",
			before, after, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), state.Controllable(&mem))
	}
	if mem.U8(sym.JoyIgnore) != 0 {
		t.Errorf("wJoyIgnore = %#04x after cutscene, want 0", mem.U8(sym.JoyIgnore))
	}
	if !state.Controllable(&mem) {
		t.Errorf("player not controllable after cutscene; map=%#04x at (%d,%d)",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
}
