package benchmark

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestProfileUsesSemanticMilestones(t *testing.T) {
	p := Profile()
	if p.Game != "pokemon-red" || len(p.Milestones) < 18 {
		t.Fatalf("profile = %+v", p)
	}
	cases := []struct {
		id  string
		obs agent.Observation
	}{
		{"starter-obtained", agent.Observation{Party: []agent.PartyMon{{Species: "squirtle", Level: 5}}}},
		{"rocket-hideout", agent.Observation{Story: agent.ProgressState{{ID: "silph_scope_acquired", Complete: true}}}},
		{"surf-obtained", agent.Observation{FieldCapabilities: []agent.FieldCapability{{Name: "surf", HMOwned: true}}}},
		{"silph-completed", agent.Observation{Story: agent.ProgressState{{ID: "silph_co_cleared", Complete: true}}}},
		{"cinnabar", agent.Observation{MapName: "CINNABAR_ISLAND"}},
		{"elite-four", agent.Observation{Story: agent.ProgressState{{ID: "league_lance_defeated", Complete: true}}}},
	}
	for _, tc := range cases {
		milestone, ok := p.Milestone(tc.id)
		if !ok {
			t.Fatalf("milestone %q missing", tc.id)
		}
		if !milestone.Reached(tc.obs) {
			t.Fatalf("milestone %q did not match semantic observation %+v", tc.id, tc.obs)
		}
	}
}

func TestGoalForComparisonSegments(t *testing.T) {
	cases := map[string]string{
		"brock":         "badges:1",
		"sabrina":       "badges:6",
		"blaine":        "badges:7",
		"hall-of-fame":  "elite-four",
		"surf-obtained": "capability:surf",
	}
	for in, want := range cases {
		got, ok := GoalFor(in)
		if !ok || got != want {
			t.Fatalf("GoalFor(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
}
