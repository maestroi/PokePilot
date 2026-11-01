package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
)

const (
	farmTrainingCorpusName = "planner-training-v1.jsonl.gz"
	trainingSchemaVersion  = 1
	// Without S3, avoid making the Finish request unbounded. S3-backed
	// artifacts can be arbitrarily larger without entering the wall JSON.
	maxInlineTrainingBytes = 4 << 20
)

// plannerTrainingExecution is only populated when the objective executed in
// this exact round matches the choice or FIRST plan step. A later success is
// not evidence that a previous plan was sound.
type plannerTrainingExecution struct {
	Round        int           `json:"round"`
	Objective    string        `json:"objective"`
	Outcome      agent.Outcome `json:"outcome"`
	Recovered    bool          `json:"recovered,omitempty"`
	Terminal     bool          `json:"terminal,omitempty"`
	BadgesBefore int           `json:"badges_before"`
	BadgesAfter  int           `json:"badges_after"`
}

// plannerTrainingExample is a raw, provenance-rich supervision candidate,
// NOT a positive reward label. Offline filtering must reject failures,
// duplicates, and outcomes that do not actually demonstrate useful progress.
type plannerTrainingExample struct {
	Version      int                          `json:"version"`
	Task         string                       `json:"task"`
	RunID        string                       `json:"run_id"`
	Attempt      int                          `json:"attempt"`
	Game         string                       `json:"game"`
	Seed         int64                        `json:"seed"`
	ROMSHA256    string                       `json:"rom_sha256"`
	Build        string                       `json:"build"`
	Goal         string                       `json:"goal,omitempty"`
	Round        int                          `json:"round"`
	Model        string                       `json:"model,omitempty"`
	Backend      string                       `json:"backend,omitempty"`
	PromptHash   string                       `json:"prompt_hash,omitempty"`
	Observation  agent.Observation            `json:"observation"`
	Offered      []string                     `json:"offered"`
	ReplanReason string                       `json:"replan_reason,omitempty"`
	PlanGoal     string                       `json:"plan_goal,omitempty"`
	PlanSteps    []string                     `json:"plan_steps,omitempty"`
	PlanBoundary bool                         `json:"plan_boundary,omitempty"`
	Choice       string                       `json:"choice,omitempty"`
	Rejected     bool                         `json:"rejected,omitempty"`
	Error        string                       `json:"error,omitempty"`
	Execution    *plannerTrainingExecution    `json:"execution,omitempty"`
}

// writeFarmTrainingCorpus exports ALL model calls for an opted-in run, rather
// than the 64 strategic records retained in the dashboard summary. A temp
// file + rename prevents the finish path from reading a partial gzip.
func writeFarmTrainingCorpus(spec farm.Spec, res agent.Result, calls []agent.LLMCall, dir, romSHA256 string) error {
	if !spec.TrainingCapture || spec.Planner != "llm" || dir == "" || len(calls) == 0 {
		return nil
	}
	f, err := os.CreateTemp(dir, ".planner-training-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)

	// The settled result and its parallel timing are the authoritative
	// execution log. Match by round AND objective; never infer success from
	// a later badge or an unrelated deterministic step.
	outcomes := make(map[int]agent.ObjectiveResult, len(res.Outcomes))
	for i, result := range res.Outcomes {
		if i < len(res.OutcomeTimings) {
			outcomes[res.OutcomeTimings[i].Round] = result
		}
	}
	zip := gzip.NewWriter(f)
	enc := json.NewEncoder(zip)
	game := spec.Game
	if game == "" {
		game = "pokemon-red"
	}
	for _, call := range calls {
		task := "objective_choice"
		if call.Strategic {
			task = "strategic_plan"
		}
		ex := plannerTrainingExample{
			Version: trainingSchemaVersion, Task: task,
			RunID: spec.RunID, Attempt: spec.Attempt, Game: game,
			Seed: spec.Seed, ROMSHA256: romSHA256, Build: version,
			Goal: spec.Goal.String(), Round: call.Observation.Round,
			Model: call.Model, Backend: call.Backend, PromptHash: call.PromptHash,
			Observation: call.Observation, Offered: call.OfferedObjectives,
			Rejected: call.Err != nil,
		}
		if call.Err != nil {
			ex.Error = call.Err.Error()
		}
		var selected string
		if call.Strategic {
			ex.ReplanReason = call.ReplanReason
			ex.PlanGoal = call.Plan.Goal
			ex.PlanSteps = call.Plan.Steps
			ex.PlanBoundary = call.Plan.Boundary
			if call.Err == nil && len(call.Plan.Steps) > 0 {
				selected = call.Plan.Steps[0]
			}
		} else if call.Err == nil {
			ex.Choice = call.Objective.String()
			selected = ex.Choice
		}
		if settled, ok := outcomes[ex.Round]; ok && selected != "" && settled.Objective.String() == selected {
			ex.Execution = &plannerTrainingExecution{
				Round: ex.Round, Objective: selected, Outcome: settled.Outcome,
				Recovered: settled.Recovered, Terminal: settled.Terminal,
				BadgesBefore: len(call.Observation.Badges),
				BadgesAfter: len(settled.Final.Badges),
			}
		}
		if err := enc.Encode(ex); err != nil {
			zip.Close()
			f.Close()
			return err
		}
	}
	if err := zip.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, farmTrainingCorpusName))
}

// appendFarmTrainingArtifact persists the optional corpus through the same
// finish artifact contract as game recordings. S3 keeps large training data
// out of Postgres; local installations get a bounded inline artifact.
func appendFarmTrainingArtifact(report *farm.FinishReport, spec farm.Spec, dir string) error {
	if report == nil || !spec.TrainingCapture || dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, farmTrainingCorpusName))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	store, configured, err := artifactstore.S3FromEnv()
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	artifact := farm.Artifact{
		Name: farmTrainingCorpusName, MediaType: "application/gzip",
		SHA256: hex.EncodeToString(sum[:]),
	}
	if configured {
		key := path.Join(path.Dir(farmRecordingObjectKey(spec)), farmTrainingCorpusName)
		obj, err := store.PutObject(context.Background(), key, "application/gzip", data)
		if err != nil {
			return fmt.Errorf("upload training corpus: %w", err)
		}
		artifact.Store = farm.ArtifactStoreS3
		artifact.Bucket = obj.Bucket
		artifact.ObjectKey = obj.Key
		artifact.Size = obj.Size
		artifact.SHA256 = obj.SHA256
	} else {
		if len(data) > maxInlineTrainingBytes {
			return fmt.Errorf("training corpus is %d bytes; S3 is required above %d bytes", len(data), maxInlineTrainingBytes)
		}
		artifact.Data = data
	}
	candidate := append(append([]farm.Artifact(nil), report.Artifacts...), artifact)
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: report.SeedBurn}); err != nil {
		return err
	}
	report.Artifacts = candidate
	return nil
}
