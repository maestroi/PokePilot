package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestLostEndlessRetryWithoutFreshObjectiveFallsBackToMajor(t *testing.T) {
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
	state, knowledge := majorWallPair(1, 20, "durable-badge")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{RunID: "campaign", Attempt: 1, Artifacts: []farm.Artifact{state, knowledge}}); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "campaign", Attempt: 1, Reason: "error", Detail: "failure after badge"}); err != nil {
		t.Fatal(err)
	}

	second, err := client.Lease(ctx)
	if err != nil || second == nil || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	// This worker has loaded the major checkpoint but disappears before it
	// writes any round-* pair for attempt 2.
	w.mu.Lock()
	w.tiles["campaign"].Status = statusRunning
	w.tiles["campaign"].lastUpdate = time.Now().Add(-time.Minute)
	w.mu.Unlock()
	if got := w.reapStale(time.Now()); len(got) != 1 || got[0] != "campaign" {
		t.Fatalf("reaped = %v", got)
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
		t.Fatalf("lost attempt resume = %+v, want attempt-1 major %s", cp, state.Name)
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

func TestEndlessSuccessorAfterErrorUsesParentMajorCheckpoint(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	majorState, majorKnowledge := majorWallPair(2, 80, "badge-two")
	exhaustEndlessCampaign(t, client, srv.URL, "campaign", "error", "late route failure", majorState, majorKnowledge)

	next := leaseQueuedSuccessor(t, w, client, "campaign")
	cp, err := client.ResumeCheckpoint(ctx, next.RunID, next.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil {
		t.Fatal("endless successor started from boot instead of the parent major checkpoint")
	}
	if cp.State.Name != majorState.Name || string(cp.State.Data) != "badge-two" {
		t.Fatalf("successor resume = %+v, want parent major %s", cp.State, majorState.Name)
	}
}

func TestEndlessSuccessorAfterFailedUsesParentMajorCheckpoint(t *testing.T) {
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
	state, knowledge := majorWallPair(1, 40, "badge-one")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "campaign", Attempt: 1, Artifacts: []farm.Artifact{state, knowledge},
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "campaign", Attempt: 1, Reason: "failed", Detail: "stuck on snorlax"}); err != nil {
		t.Fatal(err)
	}

	next := leaseQueuedSuccessor(t, w, client, "campaign")
	cp, err := client.ResumeCheckpoint(ctx, next.RunID, next.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.State.Name != state.Name {
		t.Fatalf("failed successor resume = %+v, want parent major %s", cp, state.Name)
	}
}

func TestEndlessSuccessorAfterDoneStartsFresh(t *testing.T) {
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
	state, knowledge := majorWallPair(8, 200, "champion")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "campaign", Attempt: 1, Artifacts: []farm.Artifact{state, knowledge},
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "campaign", Attempt: 1, Reason: "done"}); err != nil {
		t.Fatal(err)
	}

	next := leaseQueuedSuccessor(t, w, client, "campaign")
	cp, err := client.ResumeCheckpoint(ctx, next.RunID, next.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp != nil {
		t.Fatalf("successful campaign successor unexpectedly resumed %+v", cp)
	}
}

func TestEndlessSuccessorWithoutMajorStartsFresh(t *testing.T) {
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
	ordinary := wallResumeArtifact("round-010-frame-0000001000-goto.state", []byte("pre-gym"), "application/octet-stream")
	ordinaryK := wallResumeArtifact("round-010-frame-0000001000-goto.knowledge-v4.json", []byte(`{"intent":"pre-gym"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "campaign", Attempt: 1, Artifacts: []farm.Artifact{ordinary, ordinaryK},
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "campaign", Attempt: 1, Reason: "failed", Detail: "pre-badge skill bug"}); err != nil {
		t.Fatal(err)
	}

	next := leaseQueuedSuccessor(t, w, client, "campaign")
	cp, err := client.ResumeCheckpoint(ctx, next.RunID, next.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp != nil {
		t.Fatalf("pre-badge successor unexpectedly resumed %+v", cp)
	}
}

func TestEndlessSuccessorErrorRetryFallsBackToParentMajor(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	state, knowledge := majorWallPair(1, 40, "badge-one")
	exhaustEndlessCampaign(t, client, srv.URL, "campaign", "error", "late route failure", state, knowledge)

	next := leaseQueuedSuccessor(t, w, client, "campaign")
	if err := client.Finish(ctx, farm.FinishReport{RunID: next.RunID, Attempt: next.Attempt, Reason: "error", Detail: "same bug again"}); err != nil {
		t.Fatal(err)
	}
	retry, err := client.Lease(ctx)
	if err != nil || retry == nil || retry.RunID != next.RunID || retry.Attempt != 2 {
		t.Fatalf("successor retry lease = %+v, %v", retry, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, retry.RunID, retry.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.State.Name != state.Name {
		t.Fatalf("successor error retry = %+v, want parent major %s", cp, state.Name)
	}
}

func TestEndlessSuccessorChainFallsBackToAncestorMajor(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	state, knowledge := majorWallPair(1, 20, "durable-badge")
	exhaustEndlessCampaign(t, client, srv.URL, "campaign", "error", "failure one", state, knowledge)

	child := leaseQueuedSuccessor(t, w, client, "campaign")
	if err := client.Finish(ctx, farm.FinishReport{RunID: child.RunID, Attempt: child.Attempt, Reason: "failed", Detail: "died before a new badge"}); err != nil {
		t.Fatal(err)
	}
	grandchild := leaseQueuedSuccessor(t, w, client, child.RunID)
	cp, err := client.ResumeCheckpoint(ctx, grandchild.RunID, grandchild.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.State.Name != state.Name {
		t.Fatalf("grandchild resume = %+v, want ancestor major %s", cp, state.Name)
	}
}

func TestEndlessSuccessorResumeSurvivesWallRestart(t *testing.T) {
	dir := t.TempDir()
	dumps := filepath.Join(dir, "dumps")
	statePath := filepath.Join(dir, "wall.json")
	w := NewWall(dumps)
	w.SetStatePath(statePath)
	srv := httptest.NewServer(w.Handler())
	client := farm.NewClient(srv.URL)
	ctx := context.Background()
	state, knowledge := majorWallPair(1, 40, "badge-one")
	exhaustEndlessCampaign(t, client, srv.URL, "campaign", "failed", "stuck", state, knowledge)
	srv.Close()

	w2 := NewWall(dumps)
	w2.SetStatePath(statePath)
	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()
	client2 := farm.NewClient(srv2.URL)
	next := leaseQueuedSuccessor(t, w2, client2, "campaign")
	cp, err := client2.ResumeCheckpoint(ctx, next.RunID, next.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.State.Name != state.Name {
		t.Fatalf("restarted successor resume = %+v, want parent major %s", cp, state.Name)
	}
}

func exhaustEndlessCampaign(t *testing.T, client *farm.Client, wallURL, runID, reason, detail string, state, knowledge farm.Artifact) {
	t.Helper()
	ctx := context.Background()
	enqueueViaHTTP(t, wallURL, farm.Spec{RunID: runID, Planner: "llm", Goal: "beat the game", Endless: true})
	uploaded := false
	for {
		spec, err := client.Lease(ctx)
		if err != nil || spec == nil || spec.RunID != runID {
			t.Fatalf("lease %s = %+v, %v", runID, spec, err)
		}
		if !uploaded {
			if err := client.Checkpoint(ctx, farm.CheckpointReport{
				RunID: runID, Attempt: spec.Attempt, Artifacts: []farm.Artifact{state, knowledge},
			}); err != nil {
				t.Fatal(err)
			}
			uploaded = true
		}
		if err := client.Finish(ctx, farm.FinishReport{RunID: runID, Attempt: spec.Attempt, Reason: reason, Detail: detail}); err != nil {
			t.Fatal(err)
		}
		if reason != "error" && reason != "lost" {
			return
		}
		if spec.Attempt >= maxAttempts {
			return
		}
	}
}

func leaseQueuedSuccessor(t *testing.T, w *Wall, client *farm.Client, parentID string) *farm.Spec {
	t.Helper()
	got := getDashboard(t, w.Handler())
	var nextID string
	for _, r := range got.Runs {
		if r.RunID != parentID && r.Status == "queued" {
			nextID = r.RunID
			break
		}
	}
	if nextID == "" {
		t.Fatalf("no queued successor of %s", parentID)
	}
	spec, err := client.Lease(context.Background())
	if err != nil || spec == nil || spec.RunID != nextID || spec.Attempt != 1 {
		t.Fatalf("successor lease = %+v, %v, want %s attempt 1", spec, err, nextID)
	}
	return spec
}

func majorWallPair(badge, round int, stateData string) (farm.Artifact, farm.Artifact) {
	base := fmt.Sprintf("major-badge-%02d-round-%03d-frame-%010d-post-gym", badge, round, round*100)
	state := wallResumeArtifact(base+".state", []byte(stateData), "application/octet-stream")
	knowledge := wallResumeArtifact(base+".knowledge-v4.json", []byte(`{"intent":"continue campaign"}`), "application/json")
	return state, knowledge
}
