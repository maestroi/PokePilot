package farm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// RunContextArtifactName is the small, durable snapshot of the behavior knobs
// that shaped a run. It rides the generic artifact channel so older walls and
// runners remain wire-compatible while issue reporters can explain why the
// planner behaved differently between otherwise similar failures.
const RunContextArtifactName = "run-context.json"

// RunContext records the run configuration most useful when diagnosing
// planner/objective failures. These are intentionally the semantic behavior
// inputs, not every execution limit on Spec.
type RunContext struct {
	Planner         string `json:"planner,omitempty"`
	Starter         string `json:"starter,omitempty"`
	Dest            string `json:"dest,omitempty"`
	Goal            string `json:"goal,omitempty"`
	LLMProfile      string `json:"llm_profile,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	PlayStyle       string     `json:"play_style,omitempty"`
	Purpose         RunPurpose `json:"purpose,omitempty"`
	RiskTolerance   string     `json:"risk_tolerance,omitempty"`
	WildEncounters  string `json:"wild_encounters,omitempty"`
	Seed            int64  `json:"seed"`
}

// RunContextForSpec snapshots the behavior knobs that shaped this run. They
// are read straight off the Spec now that the Spec is the complete source of
// truth for a run's configuration.
func RunContextForSpec(spec Spec) RunContext {
	return RunContext{
		Planner:         spec.Planner,
		Starter:         spec.Starter,
		Dest:            spec.Dest,
		Goal:            spec.Goal.String(),
		LLMProfile:      spec.LLMProfile,
		ReasoningEffort: spec.ReasoningEffort,
		PlayStyle:       spec.PlayStyle,
		Purpose:         spec.Purpose,
		RiskTolerance:   spec.RiskTolerance,
		WildEncounters:  spec.WildEncounters,
		Seed:            spec.Seed,
	}
}

// NewRunContextArtifact creates a bounded inline JSON artifact suitable for a
// FinishReport. It is diagnostic-only; callers should never fail a run because
// this artifact cannot be produced or retained.
func NewRunContextArtifact(spec Spec) (Artifact, error) {
	data, err := json.Marshal(RunContextForSpec(spec))
	if err != nil {
		return Artifact{}, fmt.Errorf("encode run context: %w", err)
	}
	sum := sha256.Sum256(data)
	return Artifact{
		Name:      RunContextArtifactName,
		MediaType: "application/json",
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}, nil
}

// DecodeRunContext reads the durable run-context artifact from a finish dump.
// The bool is false for legacy reports that predate the artifact.
func DecodeRunContext(report FinishReport) (RunContext, bool, error) {
	for _, artifact := range report.Artifacts {
		if artifact.Name != RunContextArtifactName {
			continue
		}
		if len(artifact.Data) == 0 {
			return RunContext{}, false, fmt.Errorf("%s has no inline data", RunContextArtifactName)
		}
		var context RunContext
		if err := json.Unmarshal(artifact.Data, &context); err != nil {
			return RunContext{}, false, fmt.Errorf("decode %s: %w", RunContextArtifactName, err)
		}
		return context, true, nil
	}
	return RunContext{}, false, nil
}
