package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// TestGetStarter is the R5 milestone: from a freshly booted game, GetStarter
// follows Oak into the lab, takes Squirtle, wins the rival battle, and the
// north exit of Pallet Town leads to Route 1 (0x0C).
//
// StatAwareMove(romData) is the policy: FirstUsableMove loses this battle
// every time (the rival's mon spams GROWL) and is kept only as the trivial
// default.
func TestGetStarter(t *testing.T) {
	e := loadFixture(t)

	if err := skill.GetStarter(e, e.ROM(), skill.StarterSquirtle, skill.StatAwareMove(e.ROM())); err != nil {
		t.Fatalf("GetStarter: %v", err)
	}

	// Idempotent: the story is already complete, so a second call is a no-op.
	if err := skill.GetStarter(e, e.ROM(), skill.StarterSquirtle, skill.StatAwareMove(e.ROM())); err != nil {
		t.Fatalf("GetStarter (second call): %v", err)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	if !state.HasEvent(&mem, state.EventGotStarter) {
		t.Errorf("EventGotStarter not set: map=%#04x at (%d,%d) wJoyIgnore=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), mem.U8(sym.JoyIgnore))
	}
	if !state.HasEvent(&mem, state.EventBattledRivalInOaksLab) {
		t.Errorf("EventBattledRivalInOaksLab not set: map=%#04x at (%d,%d) wJoyIgnore=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), mem.U8(sym.JoyIgnore))
	}
	if c := state.DecodeParty(&mem).Count; c < 1 {
		t.Errorf("party count = %d, want >= 1", c)
	}
	if !state.Controllable(&mem) {
		t.Errorf("player not controllable: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}

	// Prove the gate is open: walk north to the measured gate position
	// (10,1) and push up. The map must flip to Route 1 (0x0C). This is the
	// point of the whole slice.
	dest, ok := skill.Place("pallet town")
	if !ok {
		t.Fatal(`Place: "pallet town" not found`)
	}
	if err := skill.GoTo(e, e.ROM(), dest); err != nil {
		t.Fatalf("GoTo pallet town: %v", err)
	}
	gatePath := []world.Step{
		world.StepRight, world.StepRight, world.StepRight,
		world.StepUp, world.StepUp, world.StepUp, world.StepUp,
		world.StepRight, world.StepRight,
		world.StepUp,
	}
	if err := skill.WalkPath(e, gatePath); err != nil {
		t.Fatalf("WalkPath to the north exit: %v", err)
	}
	if _, err := e.HoldUntil(emu.Up, 300, func(m *emu.Emu) bool {
		return m.Peek8(sym.CurMap) != 0x00
	}); err != nil {
		state.Snapshot(e, &mem)
		t.Fatalf("north exit did not lead out of Pallet Town: %v; map=%#04x at (%d,%d) wJoyIgnore=%#04x EventFollowedOakIntoLab=%v",
			err, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), state.HasEvent(&mem, state.EventFollowedOakIntoLab))
	}
	if _, err := e.StepUntil(600, func(m *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return state.Controllable(&mem)
	}); err != nil {
		state.Snapshot(e, &mem)
		t.Fatalf("not controllable on the far side of the gate: %v; map=%#04x at (%d,%d)",
			err, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
	state.Snapshot(e, &mem)
	if mem.U8(sym.CurMap) != 0x0C {
		t.Fatalf("wCurMap = %#04x, want Route 1 (0x0C): at (%d,%d)",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
}
