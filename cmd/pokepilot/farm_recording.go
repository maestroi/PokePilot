package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"unicode"

	"github.com/maestroi/pokepilot/artifactstore"
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

// uploadFarmRecording stores a recording remotely when S3 is configured.
// configured is true even for a broken/partial configuration: once the
// operator opts into object storage, a configuration or upload error must not
// silently put the large recording back into FinishReport JSON.
func uploadFarmRecording(spec farm.Spec, data []byte) (art farm.Artifact, configured bool, err error) {
	store, configured, err := artifactstore.S3FromEnv()
	if err != nil || !configured {
		return farm.Artifact{}, configured, err
	}
	obj, err := store.PutObject(context.Background(), farmRecordingObjectKey(spec), "application/octet-stream", data)
	if err != nil {
		return farm.Artifact{}, true, err
	}
	return farm.Artifact{
		Name:      farmRecordingName,
		MediaType: "application/octet-stream",
		SHA256:    obj.SHA256,
		Store:     farm.ArtifactStoreS3,
		Bucket:    obj.Bucket,
		ObjectKey: obj.Key,
		Size:      obj.Size,
	}, true, nil
}

func farmRecordingObjectKey(spec farm.Spec) string {
	// Keep keys pleasant to browse on a NAS while making collisions between
	// unusual run IDs impossible. The short hash is over the original ID, not
	// the sanitized display prefix.
	prefix := sanitizeObjectSegment(spec.RunID)
	if prefix == "" {
		prefix = "run"
	}
	if len(prefix) > 64 {
		prefix = prefix[:64]
	}
	sum := sha256.Sum256([]byte(spec.RunID))
	attempt := spec.Attempt
	if attempt < 1 {
		attempt = 1
	}
	return fmt.Sprintf("runs/%s-%s/attempt-%d/%s", prefix, hex.EncodeToString(sum[:6]), attempt, farmRecordingName)
}

func sanitizeObjectSegment(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(s) {
		allowed := r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-')
		if allowed {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), ".-")
}

// finishRunWithRecording mirrors finishRun but adds one optional durable
// .gbrun artifact and the run's structured objective-failure summary. It
// keeps diagnostic evidence best-effort: losing telemetry/recording must
// never change the gameplay result or prevent the lease from settling.
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

	failures := drainObjectiveFailureTelemetry(reason)
	if failureArtifact, err := farm.NewObjectiveFailureArtifact(failures); err != nil {
		log.Printf("farm: %s: objective failure telemetry: %v", report.RunID, err)
	} else if failureArtifact.Name != "" {
		candidate := append(append([]farm.Artifact(nil), report.Artifacts...), failureArtifact)
		if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: report.SeedBurn}); err != nil {
			log.Printf("farm: %s: omit %s: %v", report.RunID, failureArtifact.Name, err)
		} else {
			report.Artifacts = candidate
		}
	}

	if len(recording) > 0 {
		remote, configured, uploadErr := uploadFarmRecording(spec, recording)
		switch {
		case configured && uploadErr != nil:
			// Do not fall back to embedding the recording. An operator who
			// configured S3 did so specifically to keep large blobs out of
			// wall/database JSON; recording loss remains diagnostic-only.
			log.Printf("farm: %s: omit %s after S3 upload failure: %v", report.RunID, farmRecordingName, uploadErr)
		case configured:
			candidate := append(append([]farm.Artifact(nil), report.Artifacts...), remote)
			if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: report.SeedBurn}); err != nil {
				log.Printf("farm: %s: omit remote %s: %v", report.RunID, farmRecordingName, err)
			} else {
				report.Artifacts = candidate
			}
		default:
			// No S3 config preserves the existing local/small-install behavior.
			candidate := append(append([]farm.Artifact(nil), report.Artifacts...), farmRecordingArtifact(recording))
			if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: report.SeedBurn}); err != nil {
				log.Printf("farm: %s: omit %s: %v", report.RunID, farmRecordingName, err)
			} else {
				report.Artifacts = candidate
			}
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
