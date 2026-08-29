package agent_test

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
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
// checks the party gained the mon the objective asked for.
func TestExecuteStarter(t *testing.T) {
	e := loadFixture(t)

	const speciesCharmander uint8 = 0xB0 // ROM pokemon index, not dex number
	o := agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}
	if err := agent.Execute(e, e.ROM(), o); err != nil {
		t.Fatalf("Execute %s: %v", o, err)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	party := state.DecodeParty(&mem)
	if party.Count < 1 {
		t.Fatalf("party count = %d, want >= 1", party.Count)
	}
	if party.Mons[0].Species != speciesCharmander {
		t.Fatalf("lead species = %#04x, want %#04x (charmander): the objective said charmander",
			party.Mons[0].Species, speciesCharmander)
	}
}

// TestExecuteGoToPallet runs the starter objective, then walks to Pallet
// Town. MEASURED on main: Place("pallet town") resolves and the walk from
// Oak's lab back to Pallet Town works, so wCurMap must land on 0x00.
func TestExecuteGoToPallet(t *testing.T) {
	e := loadFixture(t)

	if err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}); err != nil {
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
		{agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}, "take the charmander starter"},
		{agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterSquirtle}, "take the squirtle starter"},
		{agent.Objective{Kind: agent.KindErrand}, "deliver oak's parcel"},
		{agent.Objective{Kind: agent.KindTrain, Level: 10}, "train the lead to level 10"},
		{agent.Objective{Kind: agent.KindHeal}, "heal the party"},
		{agent.Objective{Kind: agent.KindGym}, "beat the pewter gym leader"},
		{agent.Objective{Kind: agent.KindCatch, Species: 0x7B}, "catch a CATERPIE here"},
		{agent.Objective{Kind: agent.KindBuy, Item: 0x14, Qty: 3}, "buy 3 POTION"},
		{agent.Objective{Kind: 99}, "unknown kind 99"},
	}
	for _, c := range cases {
		if got := c.o.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

// TestValidateRejects is the argument safety net: every value a planner can
// supply is checked against a stated range, and an out-of-range or unknown
// value is REJECTED with a typed error — never clamped, never best-matched.
// No ROM needed: Validate is pure.
func TestValidateRejects(t *testing.T) {
	reject := []struct {
		name string
		o    agent.Objective
		want string // substring the error must carry
	}{
		{"train level 0", agent.Objective{Kind: agent.KindTrain, Level: 0}, "out of range"},
		{"train level 101", agent.Objective{Kind: agent.KindTrain, Level: 101}, "out of range"},
		{"catch unknown species", agent.Objective{Kind: agent.KindCatch, Species: 0x00}, "unknown species"},
		{"buy zero quantity", agent.Objective{Kind: agent.KindBuy, Item: 0x14, Qty: 0}, "out of range"},
		{"buy negative quantity", agent.Objective{Kind: agent.KindBuy, Item: 0x14, Qty: -1}, "out of range"},
		{"buy 150 quantity", agent.Objective{Kind: agent.KindBuy, Item: 0x14, Qty: 150}, "out of range"},
		{"buy unknown item", agent.Objective{Kind: agent.KindBuy, Item: 0x00, Qty: 1}, "unknown item"},
		{"unknown starter", agent.Objective{Kind: agent.KindStarter, Starter: skill.Starter(4)}, "unknown starter"},
	}
	for _, c := range reject {
		if err := c.o.Validate(); err == nil {
			t.Errorf("%s: Validate = nil, want an error", c.name)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not name the problem (%q)", c.name, err, c.want)
		}
	}

	// The boundaries themselves are legal: 1 and 100 train, 1 and 99 buy.
	accept := []agent.Objective{
		{Kind: agent.KindTrain, Level: 1},
		{Kind: agent.KindTrain, Level: 100},
		{Kind: agent.KindCatch, Species: 0x7B},
		{Kind: agent.KindBuy, Item: 0x14, Qty: 1},
		{Kind: agent.KindBuy, Item: 0x14, Qty: 99},
		{Kind: agent.KindStarter, Starter: skill.StarterCharmander},
		{Kind: agent.KindGoTo, Place: "pallet town"}, // no arguments, nothing to check
	}
	for _, o := range accept {
		if err := o.Validate(); err != nil {
			t.Errorf("%s: Validate = %v, want nil", o, err)
		}
	}
}

// TestSpeciesAndItemTables pins the argument vocabulary a planner can aim
// at: names resolve to ROM indexes and back, and unknown names do not.
func TestSpeciesAndItemTables(t *testing.T) {
	if id, ok := agent.SpeciesByName("CATERPIE"); !ok || id != 0x7B {
		t.Errorf("SpeciesByName(CATERPIE) = %#04x, %v; want 0x7B", id, ok)
	}
	if name, ok := agent.SpeciesName(0x24); !ok || name != "pidgey" {
		t.Errorf("SpeciesName(0x24) = %q, %v; want pidgey", name, ok)
	}
	if _, ok := agent.SpeciesByName("snorlax"); ok {
		t.Error("SpeciesByName(snorlax) resolved; it is not in the table")
	}
	if id, ok := agent.ItemByName("potion"); !ok || id != 0x14 {
		t.Errorf("ItemByName(potion) = %#04x, %v; want 0x14", id, ok)
	}
	if name, ok := agent.ItemName(0x0B); !ok || name != "antidote" {
		t.Errorf("ItemName(0x0B) = %q, %v; want antidote", name, ok)
	}
	if _, ok := agent.ItemByName("master ball"); ok {
		t.Error("ItemByName(master ball) resolved; it is not in the table")
	}
}
