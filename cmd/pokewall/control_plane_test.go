package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestRebindPostgresQuery(t *testing.T) {
	got := rebindPostgresQuery(`SELECT row_json FROM runs WHERE run_id=? AND status=? LIMIT -1 OFFSET ?`)
	want := `SELECT row_json FROM runs WHERE run_id=$1 AND status=$2 LIMIT ALL OFFSET $3`
	if got != want {
		t.Fatalf("rebind = %q, want %q", got, want)
	}
}

func TestControlPlaneMigrationCoversDurableSources(t *testing.T) {
	for _, table := range []string{
		"schema_migrations", "runs", "control_plane_state", "run_attempts",
		"experiments", "experiment_runs", "model_deployments", "model_hosts",
		"llm_exchanges", "objective_failures", "issue_fingerprints",
		"issue_occurrences", "issue_links", "issue_outbox", "artifacts",
		"dataset_manifests",
	} {
		if !strings.Contains(controlPlaneMigration001, "TABLE IF NOT EXISTS "+table) {
			t.Fatalf("migration missing %s", table)
		}
	}
}

func TestControlPlaneMigrationMakesFarmDeploymentDynamic(t *testing.T) {
	for _, want := range []string{
		"discover_model BOOLEAN",
		"is_default BOOLEAN",
		"endpoint_token_env TEXT",
		"max_parallel_workers INTEGER",
		"WHERE id='qwen38-27b-7900'",
		"model_id=''",
		"api_model=''",
		"max_parallel_workers=4",
	} {
		if !strings.Contains(controlPlaneMigration002, want) {
			t.Fatalf("migration 2 missing %q", want)
		}
	}
}

func TestSanitizeFinishReportKeepsMetadataNotPayloadBytes(t *testing.T) {
	in := farm.FinishReport{
		RunID: "run-1", SaveState: []byte("state"), FramePNG: []byte("png"),
		Artifacts: []farm.Artifact{{
			Name: "run.gbrun", MediaType: "application/octet-stream", SHA256: "abc",
			Data: []byte("large"), Store: "s3", Bucket: "pokepilot", ObjectKey: "runs/1/run.gbrun", Size: 5,
		}},
	}
	got := sanitizeFinishReport(in)
	if len(got.SaveState) != 0 || len(got.FramePNG) != 0 || len(got.Artifacts[0].Data) != 0 {
		t.Fatal("large payload bytes leaked into PostgreSQL report snapshot")
	}
	if got.Artifacts[0].Store != "s3" || got.Artifacts[0].ObjectKey == "" || got.Artifacts[0].SHA256 != "abc" {
		t.Fatalf("artifact metadata lost: %+v", got.Artifacts[0])
	}
}

func TestExperimentPersistWriteOrderIsStable(t *testing.T) {
	state := modelExperimentState{
		Runs: map[string]runExperimentMeta{
			"z-run": {RunID: "z-run"},
			"a-run": {RunID: "a-run"},
			"m-run": {RunID: "m-run"},
		},
		Experiments: map[string]experimentRecord{
			"exp-z": {ID: "exp-z"},
			"exp-a": {ID: "exp-a"},
		},
	}
	runs, experiments := experimentPersistWriteOrder(state)
	if want := []string{"a-run", "m-run", "z-run"}; !slices.Equal(runs, want) {
		t.Fatalf("run persist order = %v, want %v", runs, want)
	}
	if want := []string{"exp-a", "exp-z"}; !slices.Equal(experiments, want) {
		t.Fatalf("experiment persist order = %v, want %v", experiments, want)
	}
}
