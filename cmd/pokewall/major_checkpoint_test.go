package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestEndlessErrorRetryUsesLatestMajorCheckpoint(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "campaign", Planner: "llm", Goal: "beat the game", Endless: true})

	first, err := client.Lease(ctx)
	if err != nil || first == nil || first.Attempt != 1 {
		t.Fatalf("lease 1 = %+v, %v", first, err)
	}
	major1State, major1Knowledge := majorWallPair(1, 40, "badge-one")
	major2State, major2Knowledge := majorWallPair(2, 80, "badge-two")
	newerOrdinaryState := wallResumeArtifact("round-099-frame-0000009900-go-to-route-9.state", []byte("later-risky-state"), "application/octet-stream")
	newerOrdinaryKnowledge := wallResumeArtifact("round-099-frame-0000009900-go-to-route-9.knowledge-v4.json", []byte(`{"intent":"later"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "campaign", Attempt: 1,
		Artifacts: []farm.Artifact{
			major1State, major1Knowledge,
			major2State, major2Knowledge,
			newerOrdinaryState, newerOrdinaryKnowledge,
		},
	}); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "campaign", Attempt: 1, Reason: "error", Detail: "late route failure"}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	second, err := client.Lease(ctx)
	if err != nil || second == nil || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "campaign", second.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil {
		t.Fatal("endless error retry did not receive a major checkpoint")
	}
	if cp.Attempt != 1 {
		t.Fatalf("source attempt = %d, want 1", cp.Attempt)
	}
	if cp.State.Name != major2State.Name || string(cp.State.Data) != "badge-two" {
		t.Fatalf("resume state = %+v, want latest major %s", cp.State, major2State.Name)
	}
	if cp.State.Name == newerOrdinaryState.Name {
		t.Fatal("error retry used the risky latest objective instead of a major checkpoint")
	}
}

func TestEndlessMajorCheckpointFallsBackAcrossAttempts(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "campaign", Planner: "llm", Goal: "beat the game", Endless: true})

	first, err := client.Lease(ctx)
	if err != nil || first == nil {
		t.Fatalf("lease 1 = %+v, %v", first, err)
	}
	state, knowledge := majorWallPair(1, 20, "durable-badge")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{RunID: "campaign", Attempt: 1, Artifacts: []farm.Artifact{state, knowledge}}); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "campaign", Attempt: 1, Reason: "error", Detail: "failure one"}); err != nil {
		t.Fatal(err)
	}
	second, err := client.Lease(ctx)
	if err != nil || second == nil || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "campaign", Attempt: 2, Reason: "error", Detail: "failure two before upload"}); err != nil {
		t.Fatal(err)
	}

	third, err := client.Lease(ctx)
	if err != nil || third == nil || third.Attempt != 3 {
		t.Fatalf("lease 3 = %+v, %v", third, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "campaign", third.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.Attempt != 1 || cp.State.Name != state.Name {
		t.Fatalf("attempt 3 resume = %+v, want attempt-1 major %s", cp, state.Name)
	}
}

func TestMajorCheckpointRetentionHasIndependentThreeBadgeRing(t *testing.T) {
	dir := t.TempDir()
	for badge := 1; badge <= checkpointMajorKeep+2; badge++ {
		state, knowledge := majorWallPair(badge, badge*10, fmt.Sprintf("badge-%d", badge))
		if err := os.WriteFile(filepath.Join(dir, state.Name), state.Data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, knowledge.Name), knowledge.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Ordinary churn has its own ring and must not consume major slots.
	for i := 1; i <= checkpointObjectiveKeep+2; i++ {
		base := fmt.Sprintf("round-%03d-frame-%010d-goto", i, i*100)
		if err := os.WriteFile(filepath.Join(dir, base+".state"), []byte("state"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, base+".knowledge-v4.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := retainCheckpointWindow(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var majors []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), majorCheckpointPrefix) && strings.HasSuffix(e.Name(), ".state") {
			majors = append(majors, e.Name())
		}
	}
	if len(majors) != checkpointMajorKeep {
		t.Fatalf("major states kept = %d, want %d: %v", len(majors), checkpointMajorKeep, majors)
	}
	if strings.Contains(strings.Join(majors, " "), "major-badge-01-") || strings.Contains(strings.Join(majors, " "), "major-badge-02-") {
		t.Fatalf("oldest major checkpoints were not evicted: %v", majors)
	}
	if !strings.Contains(strings.Join(majors, " "), "major-badge-05-") {
		t.Fatalf("newest major checkpoint missing: %v", majors)
	}
}

func majorWallPair(badge, round int, stateData string) (farm.Artifact, farm.Artifact) {
	base := fmt.Sprintf("major-badge-%02d-round-%03d-frame-%010d-post-gym", badge, round, round*100)
	state := wallResumeArtifact(base+".state", []byte(stateData), "application/octet-stream")
	knowledge := wallResumeArtifact(base+".knowledge-v4.json", []byte(`{"intent":"continue campaign"}`), "application/json")
	return state, knowledge
}
