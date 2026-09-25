package farm

import (
	"encoding/json"
	"fmt"
	"testing"
)

func agreed(v bool) *bool { return &v }

func TestDecisionSummaryObserve(t *testing.T) {
	var s DecisionSummary
	// Two confident shadow answers (one agrees), one unsure disagreement, one
	// transport error, one active call.
	s.Observe(TypedDecisionRecord{Kind: "objective_selection", Choice: "2", ChoiceLabel: "go to pewter city", Confidence: 0.92, DurationSeconds: 0.08, Shadow: true, Executed: "go to pewter city", Agreed: agreed(true), PromptTokens: 10})
	s.Observe(TypedDecisionRecord{Kind: "objective_selection", Choice: "1", ChoiceLabel: "heal", Confidence: 0.95, DurationSeconds: 0.15, Shadow: true, Executed: "go to pewter city", Agreed: agreed(false)})
	s.Observe(TypedDecisionRecord{Kind: "objective_selection", Choice: "1", ChoiceLabel: "heal", Confidence: 0.41, DurationSeconds: 0.3, Shadow: true, Executed: "go to route 2", Agreed: agreed(false)})
	s.Observe(TypedDecisionRecord{Kind: "objective_selection", Error: "timeout", Fallback: true, DurationSeconds: 9, Shadow: true, Executed: "go to route 2"})
	s.Observe(TypedDecisionRecord{Kind: "failure_recovery", Choice: "retry", ChoiceLabel: "retry", Confidence: 0.7, DurationSeconds: 0.05})

	obj := s.Kinds["objective_selection"]
	if obj.Calls != 4 || obj.Shadow != 4 || obj.Agreements != 1 || obj.Disagreements != 2 || obj.Errors != 1 || obj.Fallbacks != 1 {
		t.Fatalf("objective counts = %+v", obj)
	}
	// Confidence counts exclude errored calls; the verdict tallies only
	// answers that could be compared.
	if obj.Confidence[9] != 2 || obj.Confidence[4] != 1 || obj.Confidence[0] != 0 {
		t.Fatalf("confidence = %v", obj.Confidence)
	}
	if obj.ConfidenceJudged[9] != 2 || obj.ConfidenceAgreed[9] != 1 || obj.ConfidenceJudged[4] != 1 || obj.ConfidenceAgreed[4] != 0 {
		t.Fatalf("calibration judged=%v agreed=%v", obj.ConfidenceJudged, obj.ConfidenceAgreed)
	}
	if obj.EngineChoices["heal"] != 2 || obj.EngineChoices["go to pewter city"] != 1 || obj.ExecutedChoices["go to route 2"] != 2 {
		t.Fatalf("choices engine=%v executed=%v", obj.EngineChoices, obj.ExecutedChoices)
	}
	// 0.08, 0.15, 0.3 and the 9s overflow: p50 lands in the 0.2s bucket,
	// p95 in the overflow bucket, reported as the last edge.
	if obj.P50Seconds != 0.2 || obj.P95Seconds != DecisionLatencyEdges[len(DecisionLatencyEdges)-1] {
		t.Fatalf("p50=%v p95=%v latency=%v", obj.P50Seconds, obj.P95Seconds, obj.Latency)
	}
	if rec := s.Kinds["failure_recovery"]; rec.Calls != 1 || rec.Shadow != 0 || rec.Agreements+rec.Disagreements != 0 {
		t.Fatalf("recovery = %+v", rec)
	}
}

func TestDecisionSummaryStaysBounded(t *testing.T) {
	var s DecisionSummary
	for i := range 10000 {
		s.Observe(TypedDecisionRecord{Kind: "objective_selection", ChoiceLabel: fmt.Sprintf("go to place %d", i), Confidence: 0.9, Shadow: true, Executed: fmt.Sprintf("go to place %d", i+1), Agreed: agreed(false)})
	}
	obj := s.Kinds["objective_selection"]
	if len(obj.EngineChoices) > maxDecisionChoiceKeys+1 || len(obj.ExecutedChoices) > maxDecisionChoiceKeys+1 {
		t.Fatalf("choice tallies grew: %d/%d", len(obj.EngineChoices), len(obj.ExecutedChoices))
	}
	if obj.EngineChoices[DecisionChoiceOther] != 10000-maxDecisionChoiceKeys {
		t.Fatalf("other = %d", obj.EngineChoices[DecisionChoiceOther])
	}
	b, err := json.Marshal(&s)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 4096 {
		t.Fatalf("summary of 10000 decisions is %d bytes", len(b))
	}
}

func TestDecisionSummaryCloneIsIndependent(t *testing.T) {
	var s DecisionSummary
	s.Observe(TypedDecisionRecord{Kind: "objective_selection", ChoiceLabel: "heal", Confidence: 0.9})
	c := s.Clone()
	s.Observe(TypedDecisionRecord{Kind: "objective_selection", ChoiceLabel: "heal", Confidence: 0.9})
	if got := c.Kinds["objective_selection"]; got.Calls != 1 || got.EngineChoices["heal"] != 1 {
		t.Fatalf("clone changed with the source: %+v", got)
	}
	var nilSummary *DecisionSummary
	if nilSummary.Clone() != nil {
		t.Fatal("nil clone")
	}
}
