package agent_test

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

// TestOfferDropsSatisfiedStarter: "take a starter" must leave the menu
// once there is a party. GetStarter is idempotent, so leaving it there
// lets the planner pick it forever, succeed instantly every time, and
// stall the run having achieved nothing — which is exactly what a live
// run did before this existed.
func TestOfferDropsSatisfiedStarter(t *testing.T) {
	all := []agent.Objective{
		{Kind: agent.KindStarter},
		{Kind: agent.KindGoTo, Place: "pallet town"},
	}

	before := agent.Offer(agent.Observation{PartyCount: 0}, all)
	if len(before) != 2 {
		t.Fatalf("with no party, Offer = %v, want both objectives", before)
	}

	after := agent.Offer(agent.Observation{PartyCount: 1}, all)
	if len(after) != 1 || after[0].Kind != agent.KindGoTo {
		t.Fatalf("with a party, Offer = %v, want the starter dropped", after)
	}
}

// TestOfferKeepsUnwiseObjectives: Offer says what is POSSIBLE, never what
// is WISE. Walking somewhere underlevelled stays on the menu; choosing it
// is the planner's mistake to make.
func TestOfferKeepsUnwiseObjectives(t *testing.T) {
	all := []agent.Objective{{Kind: agent.KindGoTo, Place: "pewter gym"}}
	got := agent.Offer(agent.Observation{
		PartyCount: 1,
		Party:      []agent.PartyMon{{Species: 7, Level: 5, HP: 1, MaxHP: 20}},
	}, all)
	if len(got) != 1 {
		t.Fatalf("Offer = %v, want the gym still offered to a hurt level-5 party", got)
	}
}
