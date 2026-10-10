package farm

import "testing"

func TestLLMStatsWithPriorAddsAttemptCounters(t *testing.T) {
	prior := (&LLMStats{
		Calls: 10, Rejected: 2, AvgSeconds: 4, StrategicCalls: 100, StrategicSeconds: 1000,
		PromptTokens: 5000, ReplanReasons: map[string]int{"blackout": 3},
		StrategicRecords: []StrategicCallRecord{{}}, PlanGoal: "old",
	}).Carried()
	if prior.StrategicRecords != nil {
		t.Fatal("Carried kept the live strategic feed")
	}
	live := &LLMStats{
		Calls: 30, Rejected: 1, AvgSeconds: 8, StrategicCalls: 38, StrategicSeconds: 513,
		PromptTokens: 153316, ReplanReasons: map[string]int{"blackout": 18}, PlanGoal: "new", Round: 118,
	}
	got := live.WithPrior(prior)
	if got.StrategicCalls != 138 || got.StrategicSeconds != 1513 || got.Calls != 40 || got.Rejected != 3 || got.PromptTokens != 158316 {
		t.Fatalf("counters not summed: %+v", got)
	}
	if got.ReplanReasons["blackout"] != 21 || live.ReplanReasons["blackout"] != 18 {
		t.Fatalf("replan reasons: merged %v, live mutated to %v", got.ReplanReasons, live.ReplanReasons)
	}
	if got.AvgSeconds != 7 || got.StrategicAvgSeconds != 1513.0/138 {
		t.Fatalf("averages not re-weighted: avg=%v strategic=%v", got.AvgSeconds, got.StrategicAvgSeconds)
	}
	if got.PlanGoal != "new" || got.Round != 118 {
		t.Fatalf("live fields must stay the current attempt's: %+v", got)
	}
	// Folding the merged tally forward again stays cumulative, not doubled.
	next := (&LLMStats{StrategicCalls: 2, StrategicSeconds: 20}).WithPrior(got.Carried())
	if next.StrategicCalls != 140 {
		t.Fatalf("third attempt: strategic calls %d, want 140", next.StrategicCalls)
	}
}
