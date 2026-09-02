package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestParseGoal(t *testing.T) {
	cases := []struct {
		in   string
		kind GoalKind
	}{
		{"", GoalNone},
		{"elite-four", GoalEliteFour},
		{"badges:8", GoalBadges},
		{"reach:Cerulean City", GoalReach},
		{"level:25", GoalLevel},
		{"item:Potion", GoalItem},
	}
	for _, tc := range cases {
		g, err := ParseGoal(tc.in)
		if err != nil {
			t.Fatalf("ParseGoal(%q): %v", tc.in, err)
		}
		if g.Kind != tc.kind {
			t.Fatalf("ParseGoal(%q).Kind = %v, want %v", tc.in, g.Kind, tc.kind)
		}
	}
}

func TestParseGoalRejectsInvalidTargets(t *testing.T) {
	for _, in := range []string{"unknown:x", "badges:0", "badges:9", "level:101", "reach:"} {
		if _, err := ParseGoal(in); err == nil {
			t.Fatalf("ParseGoal(%q) unexpectedly succeeded", in)
		}
	}
}

func TestEliteFourRequiresChampionEvent(t *testing.T) {
	g, _ := ParseGoal("elite-four")
	obs := Observation{Badges: []string{"1", "2", "3", "4", "5", "6", "7", "8"}}
	if got := EvaluateGoal(g, obs); got.Complete {
		t.Fatal("eight badges alone completed elite-four goal")
	}
	// The spelling Observe actually writes. This line used to hold the raw
	// decomp label, which no code path produces — so the test agreed with the
	// implementation and both disagreed with reality.
	obs.Events = []string{state.EventBeatChampionRival.String()}
	if got := EvaluateGoal(g, obs); !got.Complete {
		t.Fatal("champion event did not complete elite-four goal")
	}
}

func TestEvaluateGoalUsesObservableState(t *testing.T) {
	g, _ := ParseGoal("level:25")
	obs := Observation{Party: []PartyMon{{Level: 18}, {Level: 25}}}
	if got := EvaluateGoal(g, obs); !got.Complete || got.Current != 25 {
		t.Fatalf("level status = %+v", got)
	}

	g, _ = ParseGoal("item:potion")
	obs = Observation{Bag: []Item{{Name: "POTION", Quantity: 2}}}
	if got := EvaluateGoal(g, obs); !got.Complete {
		t.Fatalf("item status = %+v", got)
	}

	g, _ = ParseGoal("reach:cerulean city")
	obs = Observation{MapName: "Cerulean City"}
	if got := EvaluateGoal(g, obs); !got.Complete {
		t.Fatalf("reach status = %+v", got)
	}
}

// The elite-four goal must complete from the SAME event spelling Observe
// writes. MEASURED 2026-09-02: EvaluateGoal compared against the raw decomp
// label "EVENT_BEAT_CHAMPION_RIVAL" while Observation.Events carries
// state.Event.String() ("BeatChampionRival"), and the flag was not decoded
// at all — so `-goal elite-four` reported "badges 8/8" and could never stop.
func TestEvaluateGoalEliteFourCompletesOnDecodedEvent(t *testing.T) {
	g, err := ParseGoal("elite-four")
	if err != nil {
		t.Fatalf("ParseGoal: %v", err)
	}

	champion := state.EventBeatChampionRival.String()
	if got := EvaluateGoal(g, Observation{Events: []string{champion}}); !got.Complete {
		t.Fatalf("elite-four not complete with %q set: %+v", champion, got)
	}

	// The flag must actually be one Observe can produce, or the predicate is
	// unreachable in a real run however well it compares strings.
	var decoded bool
	for _, e := range knownEvents {
		if e == state.EventBeatChampionRival {
			decoded = true
		}
	}
	if !decoded {
		t.Fatal("EventBeatChampionRival is not in knownEvents; Observe can never report it")
	}

	if got := EvaluateGoal(g, Observation{Badges: []string{"Boulder"}}); got.Complete {
		t.Fatalf("elite-four complete without the champion event: %+v", got)
	}
}
