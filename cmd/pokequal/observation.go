package main

import (
	"fmt"

	"github.com/maestroi/pokepilot/agent"
)

// readQualificationObservation keeps qualification failures explicit instead of
// using agent.Observe's panic-on-error compatibility wrapper. The injected
// reader also keeps the error path ROM-free and cheap to regression-test.
func readQualificationObservation(scope, phase string, observe func() (agent.Observation, error)) (agent.Observation, error) {
	obs, err := observe()
	if err != nil {
		return agent.Observation{}, fmt.Errorf("pokequal: %s %s observation: %w", scope, phase, err)
	}
	return obs, nil
}
