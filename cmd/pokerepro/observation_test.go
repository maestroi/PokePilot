package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestReadReproObservationReturnsPhaseTaggedError(t *testing.T) {
	sentinel := errors.New("decoder unavailable")
	obs, err := readReproObservation("checkpoint", func() (agent.Observation, error) {
		return agent.Observation{}, sentinel
	})
	if err == nil {
		t.Fatal("observation error was accepted")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapped sentinel", err)
	}
	if !strings.Contains(err.Error(), "pokerepro: checkpoint observation") {
		t.Fatalf("err = %q, want phase", err)
	}
	if obs != (agent.Observation{}) {
		t.Fatalf("obs = %+v, want zero observation on failure", obs)
	}
}
