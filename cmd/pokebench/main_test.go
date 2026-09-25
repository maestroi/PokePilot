package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/benchmark"
)

func TestPolicyPlannerRoutePriorityFollowsMode(t *testing.T) {
	speedrun := &policyPlanner{style: agent.PlayStyle(agent.PlayStyleSpeedrun)}
	if got := speedrun.RoutePriority(); got != agent.RoutePriorityFastest {
		t.Fatalf("speedrun route priority = %v, want fastest", got)
	}

	adventure := &policyPlanner{style: agent.PlayStyle(agent.PlayStyleAdventure)}
	if got := adventure.RoutePriority(); got != agent.RoutePriorityConservative {
		t.Fatalf("adventure route priority = %v, want conservative", got)
	}
}

func TestParseRedConfig(t *testing.T) {
	cfg, err := parseRedConfig([]string{
		"--rom", "/tmp/red.gb",
		"--from", "fresh",
		"--until", "hall-of-fame",
		"--runs", "3",
		"--seed", "10",
		"--output", "out",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.runs != 3 || cfg.seed != 10 || cfg.until != "hall-of-fame" || cfg.mode != "speedrun" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestParseSeeds(t *testing.T) {
	got, err := parseSeeds("7, 11,13")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 7 || got[2] != 13 {
		t.Fatalf("seeds = %+v", got)
	}
	if _, err := parseSeeds("7,nope"); err == nil {
		t.Fatal("invalid seed accepted")
	}
}

func TestResolveCheckpointUsesBenchmarkAndQualificationLayouts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sabrina.state"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveCheckpoint(root, "sabrina")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "sabrina.state" {
		t.Fatalf("resolved = %q", got)
	}

	if err := os.MkdirAll(filepath.Join(root, "rocket-hideout"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rocket-hideout", "start.state"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = resolveCheckpoint(root, "rocket-hideout")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(got)) != "rocket-hideout" {
		t.Fatalf("qualification checkpoint resolved = %q", got)
	}
}

func TestResolveSourceCarriesCheckpointProvenance(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "sabrina.state")
	if err := os.WriteFile(state, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	metaPath := strings.TrimSuffix(state, ".state") + ".benchmark.json"
	if err := benchmark.WriteJSON(metaPath, benchmark.CheckpointMetadata{
		Version: benchmark.ResultVersion, RunID: "origin", Commit: "abc", Game: "pokemon-red",
		ROMSHA256: "romhash", Seed: 11, Milestone: "sabrina",
	}); err != nil {
		t.Fatal(err)
	}
	source, resume, err := resolveSource(redConfig{from: "checkpoint:sabrina", corpus: root})
	if err != nil {
		t.Fatal(err)
	}
	if resume != state || source.OriginRunID != "origin" || source.OriginCommit != "abc" ||
		source.OriginMilestone != "sabrina" || source.OriginSeed != 11 {
		t.Fatalf("source = %+v resume=%q", source, resume)
	}
}

func TestParseRedConfigRejectsBehaviorChangingUnknownMode(t *testing.T) {
	if _, err := parseRedConfig([]string{"--mode", "mystery"}); err == nil {
		t.Fatal("unknown mode accepted")
	}
}
