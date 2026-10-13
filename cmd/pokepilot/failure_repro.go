package main

import (
	"log"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

// appendFailureReproArtifacts turns each structured objective failure with a
// preserved pre-objective checkpoint into a tiny fixer handoff. The handoff is
// intentionally ROM-free: cmd/pokerepro fetches the exact state/knowledge pair
// from pokewall while the fixer uses its own local ROM.
//
// The contract records the play style the run actually executed under, not
// the raw spec field: an empty spec runs as speedrun (fastest route
// priority), and recording "" made pokerepro -verify replay it with the
// conservative router, so real stalls were reported cannot_reproduce.
func appendFailureReproArtifacts(report *farm.FinishReport, failures []farm.ObjectiveFailure, playStyle string) {
	if report == nil {
		return
	}
	playStyle = agent.NormalizePlayStyle(playStyle)
	for _, failure := range failures {
		repro, err := farm.NewFailureReproArtifact(*report, failure, playStyle)
		if err != nil {
			log.Printf("farm: %s: failure repro for %s: %v", report.RunID, failure.Fingerprint, err)
			continue
		}
		if repro.Name == "" {
			continue
		}
		candidate := append(append([]farm.Artifact(nil), report.Artifacts...), repro)
		if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: report.SeedBurn}); err != nil {
			log.Printf("farm: %s: omit %s: %v", report.RunID, repro.Name, err)
			continue
		}
		report.Artifacts = candidate
	}
}
