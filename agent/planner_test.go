package agent_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

// The planner tests are pure logic: no ROM, no fixture, no emulator. They
// must run with POKEMON_RED_ROM unset.

func testObjectives() []agent.Objective {
	return []agent.Objective{
		{Kind: agent.KindStarter},
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
		{"take a starter", 0},
		{"go to viridian pokemon center", 1},
		{"GO TO VIRIDIAN POKEMON CENTER", 1},
		{"Talk at (3,1)", 2},
		{"  take a starter  ", 0},
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
