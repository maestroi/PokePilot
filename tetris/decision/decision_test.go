package decision

import (
	"context"
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/tetris"
	"github.com/maestroi/pokepilot/tetris/policy"
)

type fakeEngine struct {
	decide func(agent.DecisionRequest) (agent.DecisionResponse, error)
	req    agent.DecisionRequest
}

func (e *fakeEngine) Decide(_ context.Context, req agent.DecisionRequest) (agent.DecisionResponse, error) {
	e.req = req
	return e.decide(req)
}

func readyState(piece tetris.Piece) tetris.State {
	return tetris.State{
		Mode:  tetris.ModeA,
		Board: tetris.Board{},
		Active: &tetris.PieceState{
			Piece: piece,
			X:     4,
			Y:     0,
		},
		Next:               &tetris.PiecePreview{Piece: tetris.PieceI},
		ReadyForPieceInput: true,
	}
}

func responseFor(req agent.DecisionRequest, choice string, selected float64) agent.DecisionResponse {
	probs := make(map[string]float64, len(req.Choices))
	other := 0.0
	if len(req.Choices) > 1 {
		other = (1 - selected) / float64(len(req.Choices)-1)
	}
	for _, c := range req.Choices {
		probs[c.ID] = other
	}
	probs[choice] = selected
	return agent.DecisionResponse{
		Choice:        choice,
		Probabilities: probs,
		Backend:       "jev",
		Model:         "jev-test",
	}
}

func TestSelectorActiveCanChooseDifferentLegalPlacement(t *testing.T) {
	state := readyState(tetris.PieceT)
	deterministic, err := policy.Choose(state, policy.ObjectiveScore)
	if err != nil {
		t.Fatal(err)
	}
	deterministicID := placementID(deterministic.Candidate)
	engine := &fakeEngine{}
	engine.decide = func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		for _, choice := range req.Choices {
			if choice.ID != deterministicID {
				return responseFor(req, choice.ID, 0.8), nil
			}
		}
		t.Fatal("no alternate legal placement")
		return agent.DecisionResponse{}, nil
	}

	got, err := (Selector{Engine: engine, MinConfidence: 0.65}).Choose(context.Background(), state, policy.ObjectiveScore)
	if err != nil {
		t.Fatal(err)
	}
	if got.Fallback || got.DecisionErr != nil || got.Shadow {
		t.Fatalf("selection flags = fallback=%v err=%v shadow=%v", got.Fallback, got.DecisionErr, got.Shadow)
	}
	if placementID(got.Decision.Candidate) == deterministicID {
		t.Fatalf("active Jev choice did not replace deterministic placement %s", deterministicID)
	}
	if got.Response.Backend != "jev" || got.Response.Confidence < 0.79 {
		t.Fatalf("response = %+v", got.Response)
	}
	if got.Agreed == nil || *got.Agreed {
		t.Fatalf("agreement = %v, want false for alternate placement", got.Agreed)
	}
	if got.Request.Kind != KindPlacement || len(got.Request.Choices) != got.Decision.Considered {
		t.Fatalf("request = kind %q choices %d considered %d", got.Request.Kind, len(got.Request.Choices), got.Decision.Considered)
	}
}

func TestSelectorLowConfidenceFallsBackDeterministic(t *testing.T) {
	state := readyState(tetris.PieceL)
	deterministic, err := policy.Choose(state, policy.ObjectiveSurvival)
	if err != nil {
		t.Fatal(err)
	}
	engine := &fakeEngine{decide: func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		return responseFor(req, req.Choices[0].ID, 0.4), nil
	}}

	got, err := (Selector{Engine: engine, MinConfidence: 0.7}).Choose(context.Background(), state, policy.ObjectiveSurvival)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Fallback || !errors.Is(got.DecisionErr, agent.ErrDecisionLowConfidence) {
		t.Fatalf("fallback=%v err=%v", got.Fallback, got.DecisionErr)
	}
	if placementID(got.Decision.Candidate) != placementID(deterministic.Candidate) {
		t.Fatalf("fallback placement = %s, want deterministic %s", placementID(got.Decision.Candidate), placementID(deterministic.Candidate))
	}
}

func TestSelectorInvalidBackendChoiceFallsBack(t *testing.T) {
	state := readyState(tetris.PieceO)
	engine := &fakeEngine{decide: func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		probs := map[string]float64{"not-declared": 1}
		return agent.DecisionResponse{Choice: "not-declared", Probabilities: probs}, nil
	}}

	got, err := (Selector{Engine: engine}).Choose(context.Background(), state, policy.ObjectiveScore)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Fallback || !errors.Is(got.DecisionErr, agent.ErrInvalidDecision) {
		t.Fatalf("fallback=%v err=%v", got.Fallback, got.DecisionErr)
	}
}

func TestSelectorShadowNeverOverridesPolicy(t *testing.T) {
	state := readyState(tetris.PieceS)
	deterministic, err := policy.Choose(state, policy.ObjectiveSurvival)
	if err != nil {
		t.Fatal(err)
	}
	deterministicID := placementID(deterministic.Candidate)
	engine := &fakeEngine{}
	engine.decide = func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		for _, choice := range req.Choices {
			if choice.ID != deterministicID {
				return responseFor(req, choice.ID, 0.9), nil
			}
		}
		return responseFor(req, deterministicID, 0.9), nil
	}

	got, err := (Selector{Engine: engine, Shadow: true}).Choose(context.Background(), state, policy.ObjectiveSurvival)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Shadow || got.Fallback {
		t.Fatalf("shadow=%v fallback=%v", got.Shadow, got.Fallback)
	}
	if placementID(got.Decision.Candidate) != deterministicID {
		t.Fatalf("shadow executed %s, want deterministic %s", placementID(got.Decision.Candidate), deterministicID)
	}
	if got.Agreed == nil || *got.Agreed {
		t.Fatalf("agreement = %v, want disagreement", got.Agreed)
	}
}

func TestPlacementRequestContainsOnlyPolicyCandidates(t *testing.T) {
	state := readyState(tetris.PieceI)
	resolved, candidates, err := policy.Candidates(state, policy.ObjectiveScore)
	if err != nil {
		t.Fatal(err)
	}
	req, byID, err := placementRequest(state, resolved, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Choices) != len(candidates) || len(byID) != len(candidates) {
		t.Fatalf("choices=%d map=%d candidates=%d", len(req.Choices), len(byID), len(candidates))
	}
	for _, choice := range req.Choices {
		candidate, ok := byID[choice.ID]
		if !ok {
			t.Fatalf("choice %q has no policy candidate", choice.ID)
		}
		if choice.ID != placementID(candidate) {
			t.Fatalf("choice id %q != candidate id %q", choice.ID, placementID(candidate))
		}
	}
}
