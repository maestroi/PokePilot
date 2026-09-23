package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestLostWorkerRetryPrefersHigherFrameOverCarriedOverRoundNumber(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "carry", Planner: "llm", Goal: farm.GoalFrom("beat the game")})

	first, err := client.Lease(ctx)
	if err != nil || first == nil || first.Attempt != 1 {
		t.Fatalf("lease 1 = %+v, %v", first, err)
	}
	carriedState := frameSortResumeArtifact("round-041-frame-0001639091-talk.state", []byte("carried-state"), "application/octet-stream")
	carriedKnowledge := frameSortResumeArtifact("round-041-frame-0001639091-talk.knowledge-v4.json", []byte(`{"intent":"old"}`), "application/json")
	newerState := frameSortResumeArtifact("round-016-frame-0001829499-hm01.state", []byte("newer-state"), "application/octet-stream")
	newerKnowledge := frameSortResumeArtifact("round-016-frame-0001829499-hm01.knowledge-v4.json", []byte(`{"intent":"new"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{RunID: "carry", Attempt: 1, Artifacts: []farm.Artifact{carriedState, carriedKnowledge}}); err != nil {
		t.Fatalf("checkpoint carried: %v", err)
	}
	if err := client.Checkpoint(ctx, farm.CheckpointReport{RunID: "carry", Attempt: 1, Artifacts: []farm.Artifact{newerState, newerKnowledge}}); err != nil {
		t.Fatalf("checkpoint newer: %v", err)
	}

	w.mu.Lock()
	w.tiles["carry"].Status = statusRunning
	w.tiles["carry"].lastUpdate = time.Now().Add(-time.Minute)
	w.mu.Unlock()
	if got := w.reapStale(time.Now()); len(got) != 1 || got[0] != "carry" {
		t.Fatalf("reaped = %v", got)
	}

	second, err := client.Lease(ctx)
	if err != nil || second == nil || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "carry", second.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.State.Name != newerState.Name || string(cp.State.Data) != "newer-state" {
		t.Fatalf("resume state = %+v, want %s", cp, newerState.Name)
	}
	if cp.Knowledge == nil || cp.Knowledge.Name != newerKnowledge.Name || string(cp.Knowledge.Data) != `{"intent":"new"}` {
		t.Fatalf("resume knowledge = %+v, want %s", cp.Knowledge, newerKnowledge.Name)
	}
}

func TestObjectiveRingEvictsOldestFrameNotLowestRoundNumber(t *testing.T) {
	dir := t.TempDir()
	writeFrameSortObjectivePair(t, dir, "round-041-frame-0001639091-talk")
	for i := 1; i <= checkpointObjectiveKeep; i++ {
		writeFrameSortObjectivePair(t, dir, fmt.Sprintf("round-%03d-frame-000164000%d-goto", i, i))
	}
	if err := retainCheckpointWindow(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var states []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "round-") && strings.HasSuffix(e.Name(), ".state") {
			states = append(states, e.Name())
		}
	}
	if len(states) != checkpointObjectiveKeep {
		t.Fatalf("kept %d objective states, want %d: %v", len(states), checkpointObjectiveKeep, states)
	}
	joined := strings.Join(states, " ")
	if strings.Contains(joined, "round-041-") {
		t.Fatalf("carried-over oldest frame was not evicted: %v", states)
	}
	for i := 1; i <= checkpointObjectiveKeep; i++ {
		if !strings.Contains(joined, fmt.Sprintf("round-%03d-frame-000164000%d-goto.state", i, i)) {
			t.Fatalf("attempt's own round %d was evicted instead of the carried-over one: %v", i, states)
		}
	}
}

func writeFrameSortObjectivePair(t *testing.T, dir, base string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, base+".state"), []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+".knowledge-v4.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func frameSortResumeArtifact(name string, data []byte, mediaType string) farm.Artifact {
	sum := sha256.Sum256(data)
	return farm.Artifact{Name: name, MediaType: mediaType, SHA256: hex.EncodeToString(sum[:]), Data: data}
}
