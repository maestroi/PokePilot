package agent_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/skill"
)

func testObjectives() []agent.Objective {
	return []agent.Objective{
		{Kind: agent.KindStarter, Starter: skill.StarterCharmander},
		{Kind: agent.KindGoTo, Place: "viridian pokemon center"},
		{Kind: agent.KindTalk, X: 3, Y: 1},
	}
}

func TestScriptedPlannerOrder(t *testing.T) {
	objs := testObjectives()
	p := agent.NewScriptedPlanner(objs...)
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
	if _, err := p.Next(wrongObs, wrongOffered); !errors.Is(err, agent.ErrDone) {
		t.Fatalf("Next after exhaustion = %v, want ErrDone", err)
	}
}

func TestChosenMatches(t *testing.T) {
	objs := testObjectives()
	cases := []struct {
		s    string
		want int
	}{
		{"take the charmander starter", 0},
		{"go to viridian pokemon center", 1},
		{"GO TO VIRIDIAN POKEMON CENTER", 1},
		{"Talk at (3,1)", 2},
		{"  TAKE THE CHARMANDER STARTER  ", 0},
		{"1", 0},
		{"2", 1},
		{"3", 2},
		// A strategist reply sometimes copies the menu's own trailing
		// annotation, or invents its own gloss, treating it as part of the
		// sentence. MEASURED 2026-09-08 on a live run: both shapes rejected
		// a valid plan step until Chosen learned to strip one trailing
		// parenthetical and retry.
		{"go to viridian pokemon center  (unvisited adjacent map)", 1},
		{"take the charmander starter  (starts the run)", 0},
		// A re-ask's feedback quotes the offered menu back as "N: sentence"
		// lines, and the model sometimes echoes that numeral prefix back as
		// if it were part of its answer. MEASURED 2026-09-08 on
		// run-16om5de3gn5u21q5ehht4ga6db, round 60: the model's final ask
		// replied exactly "5: go to pewter pokemon center" and the run died
		// on that alone with no retries left, despite the sentence itself
		// being the correct, currently-offered choice.
		{"5: go to viridian pokemon center", 1},
		{"2: Talk at (3,1)", 2},
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
		// A numeral-colon prefix that does not resolve to an offered
		// sentence once stripped must still reject, not fall through to a
		// coincidental match.
		{"numeral prefix on an unoffered sentence", "9: go to atlantis"},
		// A hallucination invented whole cloth (not a near miss of
		// anything offered), matching the shape seen live: "go to pewter
		// gym" and "go to vermilion city" when only "go to pewter city"
		// was offered and neither was reachable this round.
		{"unoffered destination", "go to pewter gym, fleeing wild battles"},
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

func TestWithArgsApplies(t *testing.T) {
	i12 := 12
	q3 := 3

	got, err := agent.WithArgs(agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Level: &i12})
	if err != nil || got.Level != 12 {
		t.Errorf("WithArgs(train 10, level 12) = %s, %v; want level 12", got, err)
	}

	got, err = agent.WithArgs(
		agent.Objective{Kind: agent.KindCatch, Species: agent.SpeciesID("caterpie")},
		agent.ReplyArgs{Species: "Pidgey"},
	)
	if err != nil || got.Species != agent.SpeciesID("pidgey") {
		t.Errorf("WithArgs(catch caterpie, species Pidgey) = %s, %v; want semantic species pidgey", got, err)
	}

	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindBuy}, agent.ReplyArgs{Item: "potion", Quantity: &q3})
	if err != nil || got.Item != agent.ItemID("potion") || got.Qty != 3 {
		t.Errorf("WithArgs(buy, potion x3) = %s, %v; want semantic item potion qty 3", got, err)
	}

	fleeTrue, fleeFalse := true, false
	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindGoTo, Place: "mt moon 1f"}, agent.ReplyArgs{Flee: &fleeTrue})
	if err != nil || !got.Flee {
		t.Errorf("WithArgs(go to, flee true) = %s, %v; want Flee set", got, err)
	}
	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindHeal, Place: "viridian pokemon center"}, agent.ReplyArgs{Flee: &fleeTrue})
	if err != nil || !got.Flee {
		t.Errorf("WithArgs(heal at a place, flee true) = %s, %v; want Flee set", got, err)
	}
	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}, agent.ReplyArgs{Flee: &fleeFalse})
	if err != nil || got.Flee {
		t.Errorf("WithArgs(go to, flee false) = %s, %v; want the objective unchanged", got, err)
	}

	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}, agent.ReplyArgs{})
	if err != nil || got.Place != "pallet town" {
		t.Errorf("WithArgs with no args = %s, %v; want the objective unchanged", got, err)
	}

	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}, agent.ReplyArgs{Intent: "reach the gym"})
	if err != nil || got.Intent != "reach the gym" {
		t.Errorf("WithArgs(go to, intent) = %s, %v; want Intent set", got, err)
	}
	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Level: &i12, Intent: "earn the boulder badge"})
	if err != nil || got.Level != 12 || got.Intent != "earn the boulder badge" {
		t.Errorf("WithArgs(train, level+intent) = %s, %v; want both applied", got, err)
	}
	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}, agent.ReplyArgs{Intent: "start the journey"})
	if err != nil || got.Intent != "start the journey" {
		t.Errorf("WithArgs(starter, intent) = %s, %v; want Intent set", got, err)
	}
	atCap := strings.Repeat("a", agent.IntentCap)
	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}, agent.ReplyArgs{Intent: atCap})
	if err != nil || got.Intent != atCap {
		t.Errorf("WithArgs(intent at the cap) = %s, %v; want it accepted verbatim", got, err)
	}
}

func TestWithArgsIntentOverCapRejected(t *testing.T) {
	o := agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}
	long := strings.Repeat("a", agent.IntentCap+1)

	got, err := agent.WithArgs(o, agent.ReplyArgs{Intent: long})
	if err == nil {
		t.Fatalf("WithArgs with a %d-byte intent succeeded, want a typed rejection", len(long))
	}
	if !errors.Is(err, agent.ErrIntentTooLong) {
		t.Errorf("error = %v, want errors.Is(err, ErrIntentTooLong)", err)
	}
	if got != o {
		t.Errorf("rejected reply left the objective mutated: %s", got)
	}

	got, err = agent.WithArgs(agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Level: nil, Intent: long})
	if err == nil || !errors.Is(err, agent.ErrIntentTooLong) {
		t.Errorf("WithArgs(train, over-cap intent) = %s, %v; want ErrIntentTooLong", got, err)
	}
}

func TestWithArgsRejects(t *testing.T) {
	l500, l0 := 500, 0
	l12, q3, qNeg, q150 := 12, 3, -1, 150
	fleeTrue := true
	catchCaterpie := agent.Objective{Kind: agent.KindCatch, Species: agent.SpeciesID("caterpie")}

	cases := []struct {
		name string
		o    agent.Objective
		a    agent.ReplyArgs
		want string
	}{
		{"level above range", agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Level: &l500}, "out of range"},
		{"level zero", agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Level: &l0}, "out of range"},
		{"unknown species", catchCaterpie, agent.ReplyArgs{Species: "mewthree"}, "unknown species"},
		{"fuzzy species name", catchCaterpie, agent.ReplyArgs{Species: "caterpy"}, "unknown species"},
		{"negative quantity", agent.Objective{Kind: agent.KindBuy}, agent.ReplyArgs{Item: "potion", Quantity: &qNeg}, "out of range"},
		{"quantity above range", agent.Objective{Kind: agent.KindBuy}, agent.ReplyArgs{Item: "potion", Quantity: &q150}, "out of range"},
		{"unknown item", agent.Objective{Kind: agent.KindBuy}, agent.ReplyArgs{Item: "master ball"}, "unknown item"},
		{"level on a goto", agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}, agent.ReplyArgs{Level: &l12}, "does not apply"},
		{"species on a train", agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Species: "pidgey"}, "does not apply"},
		{"quantity on a catch", catchCaterpie, agent.ReplyArgs{Quantity: &q3}, "does not apply"},
		{"flee on a train", agent.Objective{Kind: agent.KindTrain, Level: 10}, agent.ReplyArgs{Flee: &fleeTrue}, "does not apply"},
		{"flee on a catch", catchCaterpie, agent.ReplyArgs{Flee: &fleeTrue}, "does not apply"},
		{"flee on a heal in place", agent.Objective{Kind: agent.KindHeal}, agent.ReplyArgs{Flee: &fleeTrue}, "does not apply"},
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
		if got != c.o {
			t.Errorf("%s: rejected reply still mutated the objective to %s", c.name, got)
		}
	}
}
