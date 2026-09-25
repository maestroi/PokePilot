package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var (
	ErrDecisionDisabled      = errors.New("agent: typed decision backend disabled")
	ErrInvalidDecision       = errors.New("agent: invalid typed decision")
	ErrDecisionLowConfidence = errors.New("agent: typed decision confidence below threshold")
	ErrDecisionPause         = errors.New("agent: typed decision requested pause")
	ErrDecisionImpossible    = errors.New("agent: typed decision marked objective impossible")
)

// Decision error kinds are a coarse, display-safe classification of why a
// typed decision call produced no usable answer. Raw error text can carry
// backend response bodies; the kind never does.
const (
	DecisionErrorTimeout       = "timeout"
	DecisionErrorInvalidAnswer = "invalid_answer"
	DecisionErrorLowConfidence = "low_confidence"
	DecisionErrorCredentials   = "credentials"
	DecisionErrorBackend       = "backend"
)

// DecisionErrorKind classifies a failed decision call; "" for nil.
func DecisionErrorKind(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return DecisionErrorTimeout
	case errors.Is(err, ErrDecisionLowConfidence):
		return DecisionErrorLowConfidence
	case errors.Is(err, ErrInvalidDecision):
		return DecisionErrorInvalidAnswer
	case errors.Is(err, ErrDecisionCredentialsMissing):
		return DecisionErrorCredentials
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return DecisionErrorTimeout
	}
	return DecisionErrorBackend
}

// DecisionChoice is one value a DecisionEngine is allowed to return.
// IDs are opaque backend-neutral strings; generic callers map them back to
// their own semantic value only after ValidateDecisionResponse succeeds.
type DecisionChoice struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

// DecisionRequest is a constrained question over structured state.
// State deliberately stays generic JSON so the decision layer does not grow
// game-specific dependencies as additional games are added.
type DecisionRequest struct {
	Kind         string           `json:"kind,omitempty"`
	Question     string           `json:"question"`
	State        json.RawMessage  `json:"state,omitempty"`
	Choices      []DecisionChoice `json:"choices"`
	Instructions string           `json:"instructions,omitempty"`
}

// DecisionUsage is serving metadata for one constrained decision.
// Input/OutputBytes remain useful when an OpenAI-compatible server does not
// report token counts.
type DecisionUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	InputBytes       int `json:"input_bytes,omitempty"`
	OutputBytes      int `json:"output_bytes,omitempty"`
}

// DecisionResponse is one validated typed decision. Confidence is always the
// selected choice's normalized probability after validation.
type DecisionResponse struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Duration      time.Duration      `json:"-"`
	Backend       string             `json:"backend,omitempty"`
	Model         string             `json:"model,omitempty"`
	Usage         DecisionUsage      `json:"usage,omitempty"`
	Raw           string             `json:"-"`
}

// DecisionEngine answers only from a declared choice set. Implementations may
// be local, hosted, direct-logit, or ordinary OpenAI-compatible models.
type DecisionEngine interface {
	Decide(context.Context, DecisionRequest) (DecisionResponse, error)
}

// DecisionState converts portable semantic state into the raw JSON carried by a
// DecisionRequest.
func DecisionState(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("agent: marshal typed decision state: %w", err)
	}
	return json.RawMessage(b), nil
}

// ValidateDecisionRequest rejects ambiguous or unbounded decision contracts
// before any backend sees them.
func ValidateDecisionRequest(req DecisionRequest) error {
	if strings.TrimSpace(req.Question) == "" {
		return fmt.Errorf("%w: empty question", ErrInvalidDecision)
	}
	if len(req.Choices) == 0 {
		return fmt.Errorf("%w: no choices declared", ErrInvalidDecision)
	}
	seen := make(map[string]bool, len(req.Choices))
	for i, choice := range req.Choices {
		id := strings.TrimSpace(choice.ID)
		if id == "" {
			return fmt.Errorf("%w: choice %d has an empty id", ErrInvalidDecision, i)
		}
		if seen[id] {
			return fmt.Errorf("%w: duplicate choice id %q", ErrInvalidDecision, id)
		}
		seen[id] = true
	}
	return nil
}

// ValidateDecisionResponse is the safety boundary for every backend. Even a
// server that ignores a JSON schema cannot return a choice outside the declared
// set or smuggle extra values into the probability distribution.
func ValidateDecisionResponse(req DecisionRequest, resp DecisionResponse) (DecisionResponse, error) {
	if err := ValidateDecisionRequest(req); err != nil {
		return resp, err
	}
	allowed := make(map[string]bool, len(req.Choices))
	for _, choice := range req.Choices {
		allowed[choice.ID] = true
	}
	if !allowed[resp.Choice] {
		return resp, fmt.Errorf("%w: selected choice %q is not declared", ErrInvalidDecision, resp.Choice)
	}
	if len(resp.Probabilities) != len(allowed) {
		return resp, fmt.Errorf("%w: probability distribution has %d entries, want %d", ErrInvalidDecision, len(resp.Probabilities), len(allowed))
	}

	normalized := make(map[string]float64, len(resp.Probabilities))
	sum := 0.0
	for id, probability := range resp.Probabilities {
		if !allowed[id] {
			return resp, fmt.Errorf("%w: probability returned for undeclared choice %q", ErrInvalidDecision, id)
		}
		if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 || probability > 1 {
			return resp, fmt.Errorf("%w: probability for %q is %v, want 0..1", ErrInvalidDecision, id, probability)
		}
		normalized[id] = probability
		sum += probability
	}
	for id := range allowed {
		if _, ok := normalized[id]; !ok {
			return resp, fmt.Errorf("%w: probability missing for declared choice %q", ErrInvalidDecision, id)
		}
	}
	if sum <= 0 {
		return resp, fmt.Errorf("%w: probability distribution sums to zero", ErrInvalidDecision)
	}
	for id, probability := range normalized {
		normalized[id] = probability / sum
	}

	selected := normalized[resp.Choice]
	for id, probability := range normalized {
		if probability > selected+1e-9 {
			return resp, fmt.Errorf("%w: selected %q at %.4f but %q has higher probability %.4f", ErrInvalidDecision, resp.Choice, selected, id, probability)
		}
	}
	resp.Probabilities = normalized
	resp.Confidence = selected
	return resp, nil
}

// DecideChecked makes validation part of the caller contract rather than an
// implementation convention. Backends may validate internally as well, but no
// integration point needs to trust that they did.
func DecideChecked(ctx context.Context, engine DecisionEngine, req DecisionRequest) (DecisionResponse, error) {
	if engine == nil {
		return DecisionResponse{}, ErrDecisionDisabled
	}
	if err := ValidateDecisionRequest(req); err != nil {
		return DecisionResponse{}, err
	}
	resp, err := engine.Decide(ctx, req)
	if err != nil {
		return resp, err
	}
	return ValidateDecisionResponse(req, resp)
}
