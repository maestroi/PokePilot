package farm

import "encoding/json"

// StrategicCallRecord is the durable, structured seam between model
// experiments and a future planner-training pipeline. Observation is the
// exact semantic planner observation serialized by the runner; Offered is the
// complete objective menu shown to the strategist. Execution outcomes remain
// run-owned in LLMStats (PlanExecutions, StepsSkipped, ReplanReasons and the
// final stop reason on the wall tile), so engine failures stay distinguishable
// from a rejected/bad strategist reply.
type StrategicCallRecord struct {
	Observation      json.RawMessage `json:"observation,omitempty"`
	Offered          []string        `json:"offered,omitempty"`
	ReplanReason     string          `json:"replan_reason,omitempty"`
	PlanGoal         string          `json:"plan_goal,omitempty"`
	PlanSteps        []string        `json:"plan_steps,omitempty"`
	Rejected         bool            `json:"rejected,omitempty"`
	Error            string          `json:"error,omitempty"`
	DurationSeconds  float64         `json:"duration_seconds"`
	Backend          string          `json:"backend,omitempty"`
	Model            string          `json:"model,omitempty"`
	PromptTokens     int             `json:"prompt_tokens,omitempty"`
	CompletionTokens int             `json:"completion_tokens,omitempty"`
	PrefillTPS       float64         `json:"prefill_tps,omitempty"`
	DecodeTPS        float64         `json:"decode_tps,omitempty"`
}
