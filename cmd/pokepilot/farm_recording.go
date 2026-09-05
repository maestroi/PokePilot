package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
)

const farmRecordingName = "run.gbrun"

// farmRecordingMetadata is deliberately game/controller metadata. GomeBoy
// owns deterministic emulator capture; PokePilot owns what a leased run means.
func farmRecordingMetadata(spec farm.Spec, planner, starter, dest, goal string, burn int, runnerVersion string) map[string]string {
	metadata := map[string]string{
		"run_id":         spec.RunID,
		"attempt":        strconv.Itoa(spec.Attempt),
		"runner_version": runnerVersion,
		"planner":        planner,
		"seed":           strconv.FormatInt(spec.Seed, 10),
		"seed_burn":      strconv.Itoa(burn),
	}
	if starter != "" {
		metadata["starter"] = starter
	}
	if dest != "" {
		metadata["dest"] = dest
	}
	if goal != "" {
		metadata["goal"] = goal
	}
	if spec.LLMProfile != "" {
		metadata["llm_profile"] = spec.LLMProfile
	}
	return metadata
}

func stopFarmRecording(runID string, recorder *emu.SessionRecording) []byte {
	if recorder == nil {
		return nil
	}
	data, err := recorder.Stop()
	if err != nil {
		// Recording is diagnostic evidence. Losing it must never change the
		// gameplay result or prevent the farm from settling the lease.
		log.Printf("farm: %s: stop session recording: %v", runID, err)
		return nil
	}
	return data
}

func farmRecordingArtifact(data []byte) farm.Artifact {
	sum := sha256.Sum256(data)
	return farm.Artifact{
		Name:      farmRecordingName,
		MediaType: "application/octet-stream",
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}
}

// finishRunWithRecording mirrors finishRun but adds one optional durable
// .gbrun artifact. It keeps recording best-effort: if the combined checkpoint
// plus recording payload exceeds the farm artifact budget, the recording is
// dropped and the run still finishes with its existing checkpoint evidence.
func finishRunWithRecording(m *emu.Emu, client *farm.Client, spec farm.Spec, reason, detail string, burn int, checkpointDir string, progEarly, progFinal *farm.Progress, recording []byte) {
	report := farm.FinishReport{
		RunID:         spec.RunID,
		Attempt:       spec.Attempt,
		Reason:        reason,
		Detail:        detail,
		TraceTail:     m.TraceTail(20),
		RunnerVersion: client.Version,
		SeedBurn:      burn,
		ProgressEarly: progEarly,
		ProgressFinal: progFinal,
	}
	if save, err := m.SaveState(); err == nil {
		report.SaveState = save
	} else {
		log.Printf("farm: %s: save state: %v", spec.RunID, err)
	}

	defer removeCheckpointDir(checkpointDir)
	checkpointArtifacts, err := collectCheckpointArtifacts(checkpointDir)
	if err != nil {
		log.Printf("farm: %s: collect checkpoints: %v", report.RunID, err)
	} else {
		report.Artifacts = checkpointArtifacts
	}
	if len(recording) > 0 {
		candidate := append(append([]farm.Artifact(nil), report.Artifacts...), farmRecordingArtifact(recording))
		if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: report.SeedBurn}); err != nil {
			log.Printf("farm: %s: omit %s: %v", report.RunID, farmRecordingName, err)
		} else {
			report.Artifacts = candidate
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), farmFinishTimeout)
	defer cancel()
	if err := client.Finish(ctx, report); err != nil {
		log.Printf("farm: %s: finish: %v", report.RunID, err)
		return
	}
	fmt.Printf("run %s finished: %s\n", report.RunID, report.Reason)
}
