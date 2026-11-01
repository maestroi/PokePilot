package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
)

func TestWriteFarmTrainingCorpusOptInAndDirectOutcome(t *testing.T) {
	dir := t.TempDir()
	spec := farm.Spec{
		RunID: "training-run", Attempt: 2, Game: "pokemon-red",
		Planner: "llm", Seed: 13, Goal: farm.GoalFrom("badges:1"),
	}
	move := agent.Objective{Kind: agent.KindGym}
	label := move.String()
	obs := agent.Observation{Round: 3, MapName: "PEWTER_GYM"}
	calls := []agent.LLMCall{
		{
			Observation: obs, OfferedObjectives: []string{label},
			Strategic: true, ReplanReason: "initial",
			Plan:  agent.Plan{Goal: "beat Brock", Steps: []string{label}},
			Model: "teacher", Backend: "primary", PromptHash: "hash-one",
		},
		{
			Observation: obs, OfferedObjectives: []string{label},
			Strategic: true, Err: errors.New("bad JSON"),
			Model: "teacher",
		},
		{
			Observation: agent.Observation{Round: 4}, OfferedObjectives: []string{"heal"},
			Objective: agent.Objective{Kind: agent.KindHeal},
			Model:     "teacher",
		},
	}
	res := agent.Result{
		Outcomes: []agent.ObjectiveResult{
			{Objective: move, Outcome: agent.OutcomeCompleted, Final: agent.Observation{Badges: []string{"Boulder"}}},
		},
		OutcomeTimings: []agent.ObjectiveTiming{{Round: 3}},
	}
	// Not enabled: no side effects or artifacts.
	if err := writeFarmTrainingCorpus(spec, res, calls, dir, "rom-sha"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, farmTrainingCorpusName)); !os.IsNotExist(err) {
		t.Fatalf("disabled capture created an artifact: %v", err)
	}

	spec.TrainingCapture = true
	if err := writeFarmTrainingCorpus(spec, res, calls, dir, "rom-sha"); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(dir, farmTrainingCorpusName))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	dec := json.NewDecoder(z)
	var got []plannerTrainingExample
	for {
		var example plannerTrainingExample
		if err := dec.Decode(&example); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		got = append(got, example)
	}
	if len(got) != 3 {
		t.Fatalf("got %d examples, want 3", len(got))
	}
	first := got[0]
	if first.Version != 1 || first.Task != "strategic_plan" || first.RunID != spec.RunID ||
		first.Attempt != 2 || first.Seed != 13 || first.ROMSHA256 != "rom-sha" ||
		first.Model != "teacher" || first.PromptHash != "hash-one" ||
		len(first.Offered) != 1 || first.Offered[0] != label ||
		len(first.PlanSteps) != 1 || first.PlanSteps[0] != label {
		t.Fatalf("lost source provenance or plan: %+v", first)
	}
	if first.Execution == nil || first.Execution.Outcome != agent.OutcomeCompleted ||
		first.Execution.BadgesBefore != 0 || first.Execution.BadgesAfter != 1 {
		t.Fatalf("direct execution not joined: %+v", first.Execution)
	}
	if !got[1].Rejected || got[1].Execution != nil || got[1].Error != "bad JSON" {
		t.Fatalf("rejected call inherited an execution: %+v", got[1])
	}
	if got[2].Task != "objective_choice" || got[2].Execution != nil {
		t.Fatalf("unmatched choice inherited an outcome: %+v", got[2])
	}
}

func TestAppendFarmTrainingArtifactInlineAndOff(t *testing.T) {
	for _, env := range []string{
		artifactstore.EnvS3Endpoint, artifactstore.EnvS3Bucket,
		artifactstore.EnvS3AccessKey, artifactstore.EnvS3SecretKey,
	} {
		t.Setenv(env, "")
	}
	dir := t.TempDir()
	spec := farm.Spec{RunID: "training-run", Planner: "llm", TrainingCapture: true}
	data := []byte("example gzip bytes")
	if err := os.WriteFile(filepath.Join(dir, farmTrainingCorpusName), data, 0o600); err != nil {
		t.Fatal(err)
	}
	report := &farm.FinishReport{}
	if err := appendFarmTrainingArtifact(report, spec, dir); err != nil {
		t.Fatal(err)
	}
	if len(report.Artifacts) != 1 || report.Artifacts[0].Name != farmTrainingCorpusName ||
		report.Artifacts[0].MediaType != "application/gzip" || string(report.Artifacts[0].Data) != string(data) {
		t.Fatalf("training artifact not attached: %+v", report.Artifacts)
	}
	spec.TrainingCapture = false
	other := &farm.FinishReport{}
	if err := appendFarmTrainingArtifact(other, spec, dir); err != nil || len(other.Artifacts) > 0 {
		t.Fatalf("disabled capture attached artifact: %+v %v", other.Artifacts, err)
	}
}
