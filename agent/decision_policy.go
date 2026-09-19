package agent

import (
	"context"
	"fmt"
)

const (
	DecisionKindObjectiveSelection = "objective_selection"
	DecisionKindFailureRecovery    = "failure_recovery"
)

type objectiveDecisionState struct {
	Observation Observation `json:"observation"`
	Goal        string      `json:"goal,omitempty"`
}

// ObjectiveDecisionRequest turns the already-validated objective menu into a
// typed choice. The engine never creates an Objective; it can only select an
// index from what deterministic code offered.
func ObjectiveDecisionRequest(obs Observation, offered []Objective, goal string) (DecisionRequest, error) {
	state, err := DecisionState(objectiveDecisionState{Observation: obs, Goal: goal})
	if err != nil {
		return DecisionRequest{}, err
	}
	choices := make([]DecisionChoice, 0, len(offered))
	for i, objective := range offered {
		choices = append(choices, DecisionChoice{
			ID:          fmt.Sprintf("%d", i+1),
			Label:       objective.String(),
			Description: objective.Note,
		})
	}
	instructions := "Choose the currently valid objective that best advances the run without inventing a new action."
	if goal != "" {
		instructions += " The run goal is included in state; treat it as the desired outcome, not a prescribed route."
	}
	req := DecisionRequest{
		Kind:         DecisionKindObjectiveSelection,
		Question:     "Which currently valid objective should execute next?",
		State:        state,
		Choices:      choices,
		Instructions: instructions,
	}
	return req, ValidateDecisionRequest(req)
}

// DecisionObjectivePlanner lets the existing ROM-free planner replay harness
// score a typed backend against exactly the same fixtures as the generative
// 4B/9B planner.
type DecisionObjectivePlanner struct {
	Engine        DecisionEngine
	Goal          string
	MinConfidence float64

	Last             DecisionResponse
	Calls            int
	Rejected         int
	PromptTokens     int
	CompletionTokens int
}

func (p *DecisionObjectivePlanner) Next(obs Observation, offered []Objective) (Objective, error) {
	req, err := ObjectiveDecisionRequest(obs, offered, p.Goal)
	if err != nil {
		p.Rejected++
		return Objective{}, err
	}
	resp, err := DecideChecked(context.Background(), p.Engine, req)
	p.Calls++
	p.Last = resp
	p.PromptTokens += resp.Usage.PromptTokens
	p.CompletionTokens += resp.Usage.CompletionTokens
	if err != nil {
		p.Rejected++
		return Objective{}, err
	}
	if p.MinConfidence > 0 && resp.Confidence < p.MinConfidence {
		p.Rejected++
		return Objective{}, fmt.Errorf("%w: %.3f < %.3f", ErrDecisionLowConfidence, resp.Confidence, p.MinConfidence)
	}
	return Chosen(offered, resp.Choice)
}

func (p *DecisionObjectivePlanner) Usage() (prompt, completion int) {
	return p.PromptTokens, p.CompletionTokens
}

// FailureDecisionPlanner is an optional run-level seam. Run invokes it only
// after deterministic outcome policy has already classified a failure as
// recoverable; terminal controller/ownership/unknown outcomes never reach it.
type FailureDecisionPlanner interface {
	DecideFailure(ObjectiveResult) (DecisionResponse, error)
}

func FailureDecisionRequest(result ObjectiveResult) (DecisionRequest, error) {
	state, err := DecisionState(result)
	if err != nil {
		return DecisionRequest{}, err
	}
	req := DecisionRequest{
		Kind:     DecisionKindFailureRecovery,
		Question: "What is the safest recovery disposition for this recoverable objective failure?",
		State:    state,
		Choices: []DecisionChoice{
			{ID: "retry", Label: "retry", Description: "Transient condition; trying the same objective can plausibly work after the runtime's bounded recovery."},
			{ID: "recover", Label: "recover", Description: "Repair resources, party state, or another prerequisite before revisiting the objective."},
			{ID: "replan", Label: "replan", Description: "Choose a different currently valid objective and return later if state changes."},
			{ID: "pause", Label: "pause", Description: "Evidence points to a likely runtime/code blocker that should stop this run for inspection."},
			{ID: "impossible", Label: "impossible", Description: "The objective itself appears invalid or impossible in the observed state."},
			{ID: "unknown", Label: "unknown", Description: "The structured evidence is insufficient to choose a more specific recovery class."},
		},
		Instructions: "Deterministic runtime policy has already established that this outcome is eligible for replanning. You may recommend a more conservative stop, but you cannot make a terminal controller, ownership, or unknown runtime failure retryable. Prefer unknown over invented evidence.",
	}
	return req, ValidateDecisionRequest(req)
}

// FailureDecisionStops maps only conservative typed outcomes to a stop. Retry,
// recover, replan, and unknown all continue through the existing deterministic
// quarantine/recovery policy; the model never performs recovery itself.
func FailureDecisionStops(choice string) (bool, error) {
	switch choice {
	case "retry", "recover", "replan", "unknown":
		return false, nil
	case "pause", "impossible":
		return true, nil
	default:
		return false, fmt.Errorf("%w: unknown failure disposition %q", ErrInvalidDecision, choice)
	}
}
