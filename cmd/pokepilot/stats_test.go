package main

import (
	"errors"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

// TestStatsPlannerTally is the check behind the panel: a re-picked objective
// must count as a repeat (the wander signal), and a rejected reply must
// count as a call but never as a round.
func TestStatsPlannerTally(t *testing.T) {
	var pushed int
	s := newStatsPlanner(&agent.LLMPlanner{}, nil, func(any) { pushed++ }, nil)

	pallet := agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}
	lab := agent.Objective{Kind: agent.KindGoTo, Place: "oak's lab"}
	obs := agent.Observation{Round: 4, RoundsLeft: 29}
	s.record(obs, 10, pallet, nil, time.Second)
	s.record(obs, 8, lab, nil, time.Second)
	s.record(obs, 8, pallet, nil, time.Second)
	s.record(obs, 8, agent.Objective{}, errors.New("nope"), time.Second)

	got := s.stats
	if got.Calls != 4 || got.Rounds != 3 || got.Rejected != 1 {
		t.Fatalf("calls/rounds/rejected = %d/%d/%d, want 4/3/1", got.Calls, got.Rounds, got.Rejected)
	}
	if got.Repeats != 1 {
		t.Fatalf("repeats = %d, want 1 (pallet town picked twice)", got.Repeats)
	}
	if len(got.Choices) != 2 || got.Choices[0].Objective != pallet.String() || got.Choices[0].Count != 2 {
		t.Fatalf("choices = %+v, want %q x2 first", got.Choices, pallet)
	}
	if got.AvgOffered != 8.5 {
		t.Fatalf("avg offered = %v, want 8.5", got.AvgOffered)
	}
	if pushed != 4 {
		t.Fatalf("pushed %d times, want one per ask", pushed)
	}
}

// TestStatsPlannerPushesToSnap: the same tally the watch page renders is
// what the heartbeat carries, so the console and the runner's own page show
// one number. A sample tick between asks must not blank it, and a new lease
// must clear it.
func TestStatsPlannerPushesToSnap(t *testing.T) {
	snap := &heartbeatSnap{}
	snap.store(farm.Heartbeat{RunID: "r1"})
	s := newStatsPlanner(&agent.LLMPlanner{}, nil, nil, snap)

	pallet := agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}
	obs := agent.Observation{Round: 2, RoundsLeft: 30}
	s.record(obs, 6, pallet, nil, time.Second)
	s.record(obs, 6, pallet, nil, time.Second)

	hb := snap.load()
	if hb.Stats == nil {
		t.Fatal("heartbeat carries no stats after two asks")
	}
	if hb.Stats.Round != 2 || hb.Stats.Rounds != 2 || hb.Stats.Repeats != 1 {
		t.Fatalf("stats = %+v, want round 2, rounds 2, repeats 1", hb.Stats)
	}
	if len(hb.Stats.Choices) != 1 || hb.Stats.Choices[0].Count != 2 {
		t.Fatalf("choices = %+v, want %q x2", hb.Stats.Choices, pallet)
	}

	// A sample tick between asks must not blank the tally.
	snap.storeStatus(farm.Heartbeat{RunID: "r1", Frame: 9})
	if hb = snap.load(); hb.Stats == nil || hb.Stats.Rounds != 2 {
		t.Fatalf("status tick blanked the stats: %+v", hb.Stats)
	}

	// A new lease clears it.
	snap.store(farm.Heartbeat{RunID: "r2"})
	if hb = snap.load(); hb.Stats != nil {
		t.Fatalf("new lease kept the old run's stats: %+v", hb.Stats)
	}
}
