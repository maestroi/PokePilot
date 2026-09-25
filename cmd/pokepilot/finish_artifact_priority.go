package main

import (
	"fmt"

	"github.com/maestroi/pokepilot/farm"
)

// appendPriorityFinishArtifact guarantees that small structured telemetry can
// survive a finish report whose inline evidence budget is already full. When
// necessary it evicts the largest lower-priority inline artifacts first while
// preserving remote references and run-context telemetry.
func appendPriorityFinishArtifact(report *farm.FinishReport, priority farm.Artifact) ([]string, error) {
	if report == nil {
		return nil, fmt.Errorf("nil finish report")
	}
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{
		Artifacts: []farm.Artifact{priority},
		SeedBurn:  report.SeedBurn,
	}); err != nil {
		return nil, fmt.Errorf("priority artifact %s: %w", priority.Name, err)
	}
	for _, artifact := range report.Artifacts {
		if artifact.Name == priority.Name {
			return nil, fmt.Errorf("duplicate priority artifact %q", priority.Name)
		}
	}

	buildCandidate := func(removed map[int]bool) []farm.Artifact {
		candidate := make([]farm.Artifact, 0, len(report.Artifacts)+1)
		for i, artifact := range report.Artifacts {
			if removed[i] {
				continue
			}
			candidate = append(candidate, artifact)
		}
		candidate = append(candidate, priority)
		return candidate
	}

	removed := map[int]bool{}
	candidate := buildCandidate(removed)
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: report.SeedBurn}); err == nil {
		report.Artifacts = candidate
		return nil, nil
	}

	var evicted []string
	for {
		largestIdx := -1
		largestSize := -1
		for i, artifact := range report.Artifacts {
			if removed[i] || artifact.Store != "" || artifact.Name == farm.RunContextArtifactName {
				continue
			}
			size := len(artifact.Data)
			if size > largestSize || (size == largestSize && largestIdx >= 0 && artifact.Name < report.Artifacts[largestIdx].Name) {
				largestIdx = i
				largestSize = size
			}
		}
		if largestIdx < 0 {
			return evicted, fmt.Errorf("cannot fit priority artifact %q within finish artifact budget", priority.Name)
		}
		removed[largestIdx] = true
		evicted = append(evicted, report.Artifacts[largestIdx].Name)
		candidate = buildCandidate(removed)
		if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: report.SeedBurn}); err == nil {
			report.Artifacts = candidate
			return evicted, nil
		}
	}
}
