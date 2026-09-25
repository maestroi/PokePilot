package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestLostWorkerRetryOffersLatestConsistentCheckpoint(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "long-run", Planner: "llm", Goal: farm.GoalFrom("beat the game")})

	first, err := client.Lease(ctx)
	if err != nil || first == nil || first.Attempt != 1 {
		t.Fatalf("lease 1 = %+v, %v", first, err)
	}
	oldState := wallResumeArtifact("round-002-frame-0000000200-goto.state", []byte("old-state"), "application/octet-stream")
	oldKnowledge := wallResumeArtifact("round-002-frame-0000000200-goto.knowledge-v4.json", []byte(`{"intent":"old"}`), "application/json")
	newState := wallResumeArtifact("round-009-frame-0000000900-goto.state", []byte("new-state"), "application/octet-stream")
	newKnowledge := wallResumeArtifact("round-009-frame-0000000900-goto.knowledge-v4.json", []byte(`{"intent":"leave pewter"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "long-run", Attempt: 1,
		Artifacts: []farm.Artifact{oldState, oldKnowledge, newState, newKnowledge},
	}); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	w.mu.Lock()
	w.tiles["long-run"].Status = statusRunning
	w.tiles["long-run"].lastUpdate = time.Now().Add(-time.Minute)
	w.mu.Unlock()
	if got := w.reapStale(time.Now()); len(got) != 1 || got[0] != "long-run" {
		t.Fatalf("reaped = %v", got)
	}

	second, err := client.Lease(ctx)
	if err != nil || second == nil || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "long-run", second.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.Attempt != 1 {
		t.Fatalf("resume = %+v", cp)
	}
	if cp.State.Name != newState.Name || string(cp.State.Data) != "new-state" {
		t.Fatalf("state = %+v", cp.State)
	}
	if cp.Knowledge == nil || cp.Knowledge.Name != newKnowledge.Name || string(cp.Knowledge.Data) != string(newKnowledge.Data) {
		t.Fatalf("knowledge = %+v", cp.Knowledge)
	}
}

func TestOrdinaryErrorRetryResumesLatestMajorCheckpoint(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "buggy", Planner: "llm", Goal: farm.GoalFrom("beat the game")})

	first, err := client.Lease(ctx)
	if err != nil || first == nil || first.Attempt != 1 {
		t.Fatalf("lease 1 = %+v, %v", first, err)
	}
	major1State, major1Knowledge := majorWallPair(1, 10, "badge-one")
	major2State, major2Knowledge := majorWallPair(2, 20, "badge-two")
	laterState := wallResumeArtifact("round-099-frame-0000009900-stuck.state", []byte("near-failure"), "application/octet-stream")
	laterKnowledge := wallResumeArtifact("round-099-frame-0000009900-stuck.knowledge-v4.json", []byte(`{"intent":"stuck"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "buggy", Attempt: 1,
		Artifacts: []farm.Artifact{
			major1State, major1Knowledge,
			major2State, major2Knowledge,
			laterState, laterKnowledge,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "buggy", Attempt: 1, Reason: "error", Detail: "deterministic skill bug"}); err != nil {
		t.Fatal(err)
	}
	second, err := client.Lease(ctx)
	if err != nil || second == nil || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "buggy", second.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.Attempt != 1 {
		t.Fatalf("resume = %+v, want attempt-1 badge checkpoint", cp)
	}
	if cp.State.Name != major2State.Name || string(cp.State.Data) != "badge-two" {
		t.Fatalf("state = %+v, want latest major %s", cp.State, major2State.Name)
	}
	if cp.Knowledge == nil || cp.Knowledge.Name != major2Knowledge.Name {
		t.Fatalf("knowledge = %+v, want %s", cp.Knowledge, major2Knowledge.Name)
	}
	if cp.State.Name == laterState.Name {
		t.Fatal("ordinary error retry resumed the near-failure objective instead of the safe gym checkpoint")
	}
}

func TestOrdinaryErrorRetryWithoutBadgeStartsFresh(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "pre-gym", Planner: "llm", Goal: farm.GoalFrom("beat the game")})

	first, err := client.Lease(ctx)
	if err != nil || first == nil {
		t.Fatalf("lease 1 = %+v, %v", first, err)
	}
	state := wallResumeArtifact("round-001-frame-0000000100-goto.state", []byte("state"), "application/octet-stream")
	knowledge := wallResumeArtifact("round-001-frame-0000000100-goto.knowledge-v4.json", []byte(`{"intent":"x"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{RunID: "pre-gym", Attempt: 1, Artifacts: []farm.Artifact{state, knowledge}}); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "pre-gym", Attempt: 1, Reason: "error", Detail: "deterministic skill bug"}); err != nil {
		t.Fatal(err)
	}
	second, err := client.Lease(ctx)
	if err != nil || second == nil || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "pre-gym", second.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp != nil {
		t.Fatalf("pre-gym error retry unexpectedly resumed %+v", cp)
	}
}

func wallResumeArtifact(name string, data []byte, mediaType string) farm.Artifact {
	sum := sha256.Sum256(data)
	return farm.Artifact{Name: name, MediaType: mediaType, SHA256: hex.EncodeToString(sum[:]), Data: data}
}
