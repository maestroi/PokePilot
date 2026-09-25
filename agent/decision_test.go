package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type fixedDecisionEngine struct {
	resp DecisionResponse
	err  error
}

func (e fixedDecisionEngine) Decide(context.Context, DecisionRequest) (DecisionResponse, error) {
	return e.resp, e.err
}

func testDecisionRequest() DecisionRequest {
	return DecisionRequest{
		Kind:     "test",
		Question: "pick one",
		Choices: []DecisionChoice{
			{ID: "a", Label: "alpha"},
			{ID: "b", Label: "beta"},
		},
	}
}

func TestValidateDecisionResponseNormalizesConfidence(t *testing.T) {
	req := testDecisionRequest()
	got, err := ValidateDecisionResponse(req, DecisionResponse{
		Choice:        "b",
		Probabilities: map[string]float64{"a": 2, "b": 8},
	})
	if err == nil {
		t.Fatal("out-of-range probabilities unexpectedly accepted")
	}

	got, err = ValidateDecisionResponse(req, DecisionResponse{
		Choice:        "b",
		Probabilities: map[string]float64{"a": 0.2, "b": 0.8},
	})
	if err != nil {
		t.Fatalf("ValidateDecisionResponse: %v", err)
	}
	if got.Confidence != 0.8 || got.Probabilities["b"] != 0.8 {
		t.Fatalf("normalized response = %#v, want confidence 0.8", got)
	}
}

func TestValidateDecisionResponseRejectsChoiceOutsideSet(t *testing.T) {
	_, err := ValidateDecisionResponse(testDecisionRequest(), DecisionResponse{
		Choice:        "c",
		Probabilities: map[string]float64{"a": 0.4, "b": 0.6},
	})
	if !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("error = %v, want ErrInvalidDecision", err)
	}
}

func TestValidateDecisionResponseRejectsIncompleteOrInconsistentDistribution(t *testing.T) {
	req := testDecisionRequest()
	for name, resp := range map[string]DecisionResponse{
		"missing": {
			Choice:        "a",
			Probabilities: map[string]float64{"a": 1},
		},
		"selected is not max": {
			Choice:        "a",
			Probabilities: map[string]float64{"a": 0.2, "b": 0.8},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateDecisionResponse(req, resp); !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("error = %v, want ErrInvalidDecision", err)
			}
		})
	}
}

func TestDecideCheckedDoesNotTrustBackendValidation(t *testing.T) {
	engine := fixedDecisionEngine{resp: DecisionResponse{
		Choice:        "escape",
		Probabilities: map[string]float64{"a": 0.5, "b": 0.5},
	}}
	if _, err := DecideChecked(context.Background(), engine, testDecisionRequest()); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("error = %v, want invalid decision", err)
	}
}

func TestDecisionErrorKindClassifiesWithoutText(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{fmt.Errorf("wrap: %w", context.DeadlineExceeded), DecisionErrorTimeout},
		{fmt.Errorf("%w: 0.1 < 0.5", ErrDecisionLowConfidence), DecisionErrorLowConfidence},
		{fmt.Errorf("%w: selected choice %q is not declared", ErrInvalidDecision, "x"), DecisionErrorInvalidAnswer},
		{ErrDecisionCredentialsMissing, DecisionErrorCredentials},
		{errors.New("agent: Jev decision HTTP 500 Internal Server Error: body"), DecisionErrorBackend},
	} {
		if got := DecisionErrorKind(tc.err); got != tc.want {
			t.Errorf("DecisionErrorKind(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
