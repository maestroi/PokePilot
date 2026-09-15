package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

func TestReadQualificationObservationReturnsPhaseTaggedErrors(t *testing.T) {
	sentinel := errors.New("decoder unavailable")
	for _, phase := range []string{"initial", "final"} {
		t.Run(phase, func(t *testing.T) {
			obs, err := readQualificationObservation("rocket-hideout", phase, func() (agent.Observation, error) {
				return agent.Observation{}, sentinel
			})
			if err == nil {
				t.Fatal("observation error was accepted")
			}
			if !errors.Is(err, sentinel) {
				t.Fatalf("err = %v, want wrapped sentinel", err)
			}
			if !strings.Contains(err.Error(), "pokequal: rocket-hideout "+phase+" observation") {
				t.Fatalf("err = %q, want scope and phase", err)
			}
			if !reflect.DeepEqual(obs, agent.Observation{}) {
				t.Fatalf("obs = %+v, want zero observation on failure", obs)
			}
		})
	}
}
