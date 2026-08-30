package agent_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/skill"
)

// The planner tests are pure logic: no ROM, no fixture, no emulator. They
// must run with POKEMON_RED_ROM unset.

func testObjectives() []agent.Objective {
	return []agent.Objective{
		{Kind: agent.KindStarter, Starter: skill.StarterCharmander},
		{Kind: agent.KindGoTo, Place: "viridian pokemon center"},
		{Kind: agent.KindTalk, X: 3, Y: 1},
	}
}

// TestScriptedPlannerOrder checks that Next yields the objectives in the
// order they were given, then ErrDone, and nothing after.
func TestScriptedPlannerOrder(t *testing.T) {
	objs := testObjectives()
	p := agent.NewScriptedPlanner(objs...)

	// obs and offered are deliberately wrong here: a scripted planner
	// ignores both, and the returned objectives must still be its own.
	wrongObs := agent.Observation{Map: 0x7F, X: 99, Y: 99}
	wrongOffered := []agent.Objective{{Kind: agent.KindGoTo, Place: "atlantis"}}

	var got []agent.Objective
	for i := 0; ; i++ {
		o, err := p.Next(wrongObs, wrongOffered)
		if err != nil {
			if !errors.Is(err, agent.ErrDone) {
				t.Fatalf("Next #%d: unexpected error %v", i+1, err)
			}
			break
		}
		got = append(got, o)
	}
	if len(got) != len(objs) {
		t.Fatalf("got %d objectives %v, want %d", len(got), got, len(objs))
	}
	for i := range objs {
		if got[i] != objs[i] {
			t.Errorf("objective %d = %s, want %s", i, got[i], objs[i])
		}
	}

	// ErrDone is sticky: the list does not reset.
	if _, err := p.Next(wrongObs, wrongOffered); !errors.Is(err, agent.ErrDone) {
		t.Fatalf("Next after exhaustion = %v, want ErrDone", err)
	}
}

// TestChosenMatches checks the three accepted forms: the exact String(),
// the same sentence in another case, and a bare 1-based index.
func TestChosenMatches(t *testing.T) {
	objs := testObjectives()
	cases := []struct {
		s    string
		want int // index into objs
	}{
		{"take the charmander starter", 0},
		{"go to viridian pokemon center", 1},
		{"GO TO VIRIDIAN POKEMON CENTER", 1},
		{"Talk at (3,1)", 2},
		{"  TAKE THE CHARMANDER STARTER  ", 0},
		{"1", 0},
		{"2", 1},
		{"3", 2},
	}
	for _, c := range cases {
		got, err := agent.Chosen(objs, c.s)
		if err != nil {
			t.Errorf("Chosen(%q) error: %v", c.s, err)
			continue
		}
		if got != objs[c.want] {
			t.Errorf("Chosen(%q) = %s, want %s", c.s, got, objs[c.want])
		}
	}
}

// TestChosenRejects checks that every non-match is an error that names
// what was offered, never a guess.
func TestChosenRejects(t *testing.T) {
	objs := testObjectives()
	const offered = "go to viridian pokemon center"
	cases := []struct {
		name string
		s    string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"unknown name", "go to atlantis"},
		{"index 0", "0"},
		{"out-of-range index", "4"},
		{"near miss, one character off", "go to viridian pokebon center"},
	}
	for _, c := range cases {
		_, err := agent.Chosen(objs, c.s)
		if err == nil {
			t.Errorf("%s: Chosen(%q) = objective, want error", c.name, c.s)
			continue
		}
		if !strings.Contains(err.Error(), offered) {
			t.Errorf("%s: error does not name what was offered: %v", c.name, err)
		}
	}
}

// TestWithArgsApplies checks that a model-supplied argument lands on the
// objective: a level override changes the train target, a species name
// resolves to its ROM index, an item and quantity fill the buy.
func TestWithArgsApplies(t *testing.T) {
	i12 := 12
	q3 := 3

	got, err := agent.WithArgs(agent.Objective{Kind: agent.KindTrain, Level: 10},
		agent.ReplyArgs{Level: &i12})
	if err != nil || got.Level != 12 {
		t.Errorf("WithArgs(train 10, level 12) = %s, %v; want level 12", got, err)
	}

	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindCatch, Species: 0x7B},
		agent.ReplyArgs{Species: "Pidgey"})
	if err != nil || got.Species != 0x24 {
		t.Errorf("WithArgs(catch caterpie, species Pidgey) = %s, %v; want species 0x24", got, err)
	}

	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindBuy},
		agent.ReplyArgs{Item: "potion", Quantity: &q3})
	if err != nil || got.Item != 0x14 || got.Qty != 3 {
		t.Errorf("WithArgs(buy, potion x3) = %s, %v; want item 0x14 qty 3", got, err)
	}

	// No arguments: the objective comes back unchanged.
	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"},
		agent.ReplyArgs{})
	if err != nil || got.Place != "pallet town" {
		t.Errorf("WithArgs with no args = %s, %v; want the objective unchanged", got, err)
	}
}

// TestWithArgsRejects is the argument safety net at the planner layer:
// every value a model can supply is checked against a stated range before
// it can reach a skill. Out-of-range and unknown values are REJECTED with
// a typed error — never clamped, never best-matched — and an argument on
// the wrong kind is an error rather than silently dropped.
func TestWithArgsRejects(t *testing.T) {
	l500, l0 := 500, 0
	l12, q3, qNeg, q150 := 12, 3, -1, 150

	cases := []struct {
		name string
		o    agent.Objective
		a    agent.ReplyArgs
		want string
	}{
		{"level above range", agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Level: &l500}, "out of range"},
		{"level zero", agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Level: &l0}, "out of range"},
		{"unknown species", agent.Objective{Kind: agent.KindCatch, Species: 0x7B}, agent.ReplyArgs{Species: "mewthree"}, "unknown species"},
		{"fuzzy species name", agent.Objective{Kind: agent.KindCatch, Species: 0x7B}, agent.ReplyArgs{Species: "caterpy"}, "unknown species"},
		{"negative quantity", agent.Objective{Kind: agent.KindBuy}, agent.ReplyArgs{Item: "potion", Quantity: &qNeg}, "out of range"},
		{"quantity above range", agent.Objective{Kind: agent.KindBuy}, agent.ReplyArgs{Item: "potion", Quantity: &q150}, "out of range"},
		{"unknown item", agent.Objective{Kind: agent.KindBuy}, agent.ReplyArgs{Item: "master ball"}, "unknown item"},
		{"level on a goto", agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}, agent.ReplyArgs{Level: &l12}, "does not apply"},
		{"species on a train", agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Species: "pidgey"}, "does not apply"},
		{"quantity on a catch", agent.Objective{Kind: agent.KindCatch, Species: 0x7B}, agent.ReplyArgs{Quantity: &q3}, "does not apply"},
	}
	for _, c := range cases {
		got, err := agent.WithArgs(c.o, c.a)
		if err == nil {
			t.Errorf("%s: WithArgs = %s, want an error", c.name, got)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not name the problem (%q)", c.name, err, c.want)
		}
		// Rejection means the argument did NOT land: a clamped or partially
		// applied objective is exactly what this must not produce.
		if got != c.o {
			t.Errorf("%s: rejected reply still mutated the objective to %s", c.name, got)
		}
	}
}
