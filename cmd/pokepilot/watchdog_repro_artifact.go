package main

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

func appendWatchdogReproArtifact(report *farm.FinishReport, checkpointDir string) {
	if report == nil || checkpointDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(checkpointDir, agent.WatchdogReproArtifactName))
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("farm: %s: read %s: %v", report.RunID, agent.WatchdogReproArtifactName, err)
		}
		return
	}
	sum := sha256.Sum256(data)
	artifact := farm.Artifact{
		Name:      agent.WatchdogReproArtifactName,
		MediaType: "application/json",
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}
	candidate := append(append([]farm.Artifact(nil), report.Artifacts...), artifact)
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: report.SeedBurn}); err != nil {
		log.Printf("farm: %s: omit %s: %v", report.RunID, artifact.Name, err)
		return
	}
	report.Artifacts = candidate
}
