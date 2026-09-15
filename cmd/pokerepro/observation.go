package main

import (
	"fmt"

	"github.com/maestroi/pokepilot/agent"
)

// readReproObservation keeps replay harness failures explicit instead of using
// agent.Observe's panic-on-error compatibility wrapper. The injected reader
// makes unsupported/failed observation behavior deterministic in ROM-free tests.
func readReproObservation(phase string, observe func() (agent.Observation, error)) (agent.Observation, error) {
	obs, err := observe()
	if err != nil {
		return agent.Observation{}, fmt.Errorf("pokerepro: %s observation: %w", phase, err)
	}
	return obs, nil
}
