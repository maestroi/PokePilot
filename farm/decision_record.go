package farm

// TypedDecisionRecord is one constrained decision made by the experimental
// backend. It is intentionally independent of LLM strategic records so runs can
// compare latency, confidence and serving cost across backend types.
type TypedDecisionRecord struct {
	Kind             string             `json:"kind,omitempty"`
	Question         string             `json:"question,omitempty"`
	Choice           string             `json:"choice,omitempty"`
	Probabilities    map[string]float64 `json:"probabilities,omitempty"`
	Confidence       float64            `json:"confidence,omitempty"`
	DurationSeconds  float64            `json:"duration_seconds,omitempty"`
	Backend          string             `json:"backend,omitempty"`
	Model            string             `json:"model,omitempty"`
	PromptTokens     int                `json:"prompt_tokens,omitempty"`
	CompletionTokens int                `json:"completion_tokens,omitempty"`
	InputBytes       int                `json:"input_bytes,omitempty"`
	OutputBytes      int                `json:"output_bytes,omitempty"`
	Fallback         bool               `json:"fallback,omitempty"`
	// Shadow marks an answer that was only observed. Executed is what the
	// existing policy did instead, and Agreed compares it with Choice; it is
	// nil when the shadow answer was unusable.
	Shadow   bool   `json:"shadow,omitempty"`
	Executed string `json:"executed,omitempty"`
	Agreed   *bool  `json:"agreed,omitempty"`
	Error    string `json:"error,omitempty"`
}
