package agent_test

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// loadFixture restores the reds_bedroom fixture: a freshly booted game at a
// controllable overworld (map 0x26, Red's bedroom). It skips when
// POKEMON_RED_ROM is not set.
func loadFixture(t *testing.T) *emu.Emu {
	t.Helper()
	return fixture.Load(t, "reds_bedroom")
}

// TestExecuteStarter runs the KindStarter objective from a fresh boot and
// checks the party gained a mon.
func TestExecuteStarter(t *testing.T) {
	e := loadFixture(t)

	o := agent.Objective{Kind: agent.KindStarter}
	if err := agent.Execute(e, e.ROM(), o); err != nil {
		t.Fatalf("Execute %s: %v", o, err)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	if c := state.DecodeParty(&mem).Count; c < 1 {
		t.Fatalf("party count = %d, want >= 1", c)
	}
}

// TestExecuteGoToPallet runs the starter objective, then walks to Pallet
// Town. MEASURED on main: Place("pallet town") resolves and the walk from
// Oak's lab back to Pallet Town works, so wCurMap must land on 0x00.
func TestExecuteGoToPallet(t *testing.T) {
	e := loadFixture(t)

	if err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindStarter}); err != nil {
		t.Fatalf("Execute starter: %v", err)
	}

	o := agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}
	if err := agent.Execute(e, e.ROM(), o); err != nil {
		t.Fatalf("Execute %s: %v", o, err)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	if cur := mem.U8(sym.CurMap); cur != 0x00 {
		t.Fatalf("wCurMap = %#04x, want 0x00 (pallet town): at (%d,%d)",
			cur, mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
}

// TestExecuteUnknownPlace checks that a nonsense place name is an error that
// names the place, not a silent fallback to a default.
func TestExecuteUnknownPlace(t *testing.T) {
	e := loadFixture(t)

	const name = "atlantis"
	o := agent.Objective{Kind: agent.KindGoTo, Place: name}
	err := agent.Execute(e, e.ROM(), o)
	if err == nil {
		t.Fatalf("Execute %s: want error, got nil", o)
	}
	if !strings.Contains(err.Error(), name) {
		t.Fatalf("error does not name the place: %v", err)
	}
}

// TestString is the one test that needs no ROM: it pins the plain, stable
// one-line descriptions a planner will see.
func TestString(t *testing.T) {
	cases := []struct {
		o    agent.Objective
		want string
	}{
		{agent.Objective{Kind: agent.KindGoTo, Place: "viridian pokemon center"}, "go to viridian pokemon center"},
		{agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}, "go to pallet town"},
		{agent.Objective{Kind: agent.KindTalk, X: 3, Y: 1}, "talk at (3,1)"},
		{agent.Objective{Kind: agent.KindStarter}, "take a starter"},
		{agent.Objective{Kind: 99}, "unknown kind 99"},
	}
	for _, c := range cases {
		if got := c.o.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}
