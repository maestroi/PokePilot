package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

func TestFarmFinishFailureClassUsesTypedTransportError(t *testing.T) {
	err := fmt.Errorf("planner stopped: %w", agent.ErrTransport)
	if got := farmFinishFailureClass(err); got != farm.FinishFailureClassInferenceTransport {
		t.Fatalf("failure class = %q, want %q", got, farm.FinishFailureClassInferenceTransport)
	}
}

func TestFarmFinishFailureClassDoesNotParseErrorText(t *testing.T) {
	err := errors.New("agent: llm planner: transport failure: no route to host")
	if got := farmFinishFailureClass(err); got != "" {
		t.Fatalf("failure class = %q for untyped prose, want empty", got)
	}
}
