package farm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLLMStatsStructuredGoalJSON(t *testing.T) {
	want := LLMStats{
		GoalSummary: "badges 1/2",
		GoalCurrent: 1,
		GoalTarget:  2,
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"goal_summary"`, `"goal_current"`, `"goal_target"`} {
		if !strings.Contains(string(b), key) {
			t.Fatalf("structured goal stats missing %s: %s", key, b)
		}
	}

	var got LLMStats
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.GoalSummary != want.GoalSummary || got.GoalCurrent != want.GoalCurrent || got.GoalTarget != want.GoalTarget || got.GoalComplete {
		t.Fatalf("goal stats round trip = %+v, want %+v", got, want)
	}

	complete, err := json.Marshal(LLMStats{
		GoalSummary:  "badges 1/1",
		GoalCurrent:  1,
		GoalTarget:   1,
		GoalComplete: true,
	})
	if err != nil {
		t.Fatalf("marshal complete: %v", err)
	}
	if !strings.Contains(string(complete), `"goal_complete":true`) {
		t.Fatalf("complete goal status missing from JSON: %s", complete)
	}

	bare, err := json.Marshal(LLMStats{})
	if err != nil {
		t.Fatalf("marshal bare: %v", err)
	}
	for _, key := range []string{"goal_summary", "goal_current", "goal_target", "goal_complete"} {
		if strings.Contains(string(bare), key) {
			t.Fatalf("zero goal status should be omitted: %s", bare)
		}
	}
}
