package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestMediaRenderJobsPersistAndCreateIdempotently(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "wall.json")
	w1 := NewWall("")
	w1.SetStatePath(statePath)
	c1 := mediaRenderJobsFor(w1)

	req := farm.MediaRenderJobCreateRequest{
		Identity:    "runs/run-1/replay-broadcast.mp4",
		RunID:       "run-1",
		Attempts:    []int{1, 2},
		Mode:        "broadcast",
		ArtifactKey: "runs/run-1/replay-broadcast.mp4",
	}
	first, created, err := c1.ensure(req)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first ensure should create job")
	}
	second, created, err := c1.ensure(req)
	if err != nil {
		t.Fatal(err)
	}
	if created || second.ID != first.ID {
		t.Fatalf("idempotent ensure = created %t job %+v, first %+v", created, second, first)
	}

	w2 := NewWall("")
	w2.SetStatePath(statePath)
	c2 := mediaRenderJobsFor(w2)
	got, ok := c2.get(first.ID)
	if !ok {
		t.Fatalf("job %s missing after restart", first.ID)
	}
	if got.Identity != req.Identity || got.RunID != req.RunID || len(got.Attempts) != 2 {
		t.Fatalf("restored job = %+v", got)
	}
}

func TestMediaRenderJobLeaseRecoveryAndLifecycle(t *testing.T) {
	w := NewWall("")
	w.SetStatePath(filepath.Join(t.TempDir(), "wall.json"))
	c := mediaRenderJobsFor(w)
	job, _, err := c.ensure(farm.MediaRenderJobCreateRequest{
		Identity: "identity", RunID: "run-1", Attempts: []int{1}, Mode: "raw", ArtifactKey: "artifact",
	})
	if err != nil {
		t.Fatal(err)
	}

	claimed, ok, err := c.claim(job.ID, "worker-a")
	if err != nil || !ok {
		t.Fatalf("first claim = ok %t err %v job %+v", ok, err, claimed)
	}
	if _, ok, err := c.claim(job.ID, "worker-b"); err != nil || ok {
		t.Fatalf("competing claim = ok %t err %v, want busy", ok, err)
	}

	c.mu.Lock()
	expired := c.state.Jobs[job.ID]
	expired.LeaseExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	c.state.Jobs[job.ID] = expired
	if err := c.persistLocked(); err != nil {
		c.mu.Unlock()
		t.Fatal(err)
	}
	c.mu.Unlock()

	recovered, ok, err := c.claim(job.ID, "worker-b")
	if err != nil || !ok {
		t.Fatalf("stale claim recovery = ok %t err %v job %+v", ok, err, recovered)
	}
	if recovered.RetryCount != 1 || recovered.WorkerID != "worker-b" {
		t.Fatalf("recovered job = %+v", recovered)
	}

	total, done := 3, 1
	progressed, err := c.progress(job.ID, farm.MediaRenderJobProgressRequest{
		WorkerID: "worker-b", State: farm.MediaRenderJobRendering, Stage: "attempts", SegmentsTotal: &total, SegmentsDone: &done,
	})
	if err != nil {
		t.Fatal(err)
	}
	if progressed.SegmentsTotal != 3 || progressed.SegmentsDone != 1 || progressed.State != farm.MediaRenderJobRendering {
		t.Fatalf("progressed job = %+v", progressed)
	}

	failed, err := c.finish(job.ID, farm.MediaRenderJobFinishRequest{
		WorkerID: "worker-b", State: farm.MediaRenderJobFailed, LastError: "encoder failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.State != farm.MediaRenderJobFailed || failed.LastError != "encoder failed" {
		t.Fatalf("failed job = %+v", failed)
	}
	retried, err := c.retry(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.State != farm.MediaRenderJobQueued || retried.FinishedAt != 0 {
		t.Fatalf("retried job = %+v", retried)
	}
	cancelled, err := c.cancel(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != farm.MediaRenderJobCancelled {
		t.Fatalf("cancelled job = %+v", cancelled)
	}
	retried, err = c.retry(job.ID)
	if err != nil {
		t.Fatal(err)
	}

	claimed, ok, err = c.claim(job.ID, "worker-c")
	if err != nil || !ok {
		t.Fatalf("retry claim = ok %t err %v", ok, err)
	}
	ready, err := c.reconcileReady(job.ID, 12345)
	if err != nil {
		t.Fatal(err)
	}
	if ready.State != farm.MediaRenderJobReady || ready.ResultSize != 12345 || ready.WorkerID != "" || ready.LeaseExpiresAt != 0 {
		t.Fatalf("reconciled ready job = %+v", ready)
	}
	removed, err := c.remove(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if removed.ID != job.ID {
		t.Fatalf("removed job = %+v", removed)
	}
	if _, ok := c.get(job.ID); ok {
		t.Fatalf("job %s still present after removal", job.ID)
	}
}

func TestMediaRenderJobRemoveRejectsQueuedWork(t *testing.T) {
	w := NewWall("")
	w.SetStatePath(filepath.Join(t.TempDir(), "wall.json"))
	c := mediaRenderJobsFor(w)
	job, _, err := c.ensure(farm.MediaRenderJobCreateRequest{
		Identity: "queued", RunID: "run-q", Attempts: []int{1}, Mode: "broadcast", ArtifactKey: "queued.mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.remove(job.ID); err == nil {
		t.Fatal("remove queued job unexpectedly succeeded")
	}
	if _, ok := c.get(job.ID); !ok {
		t.Fatal("queued job disappeared after rejected removal")
	}
}

func TestMediaRenderJobsListSummarizesAndOrdersRecentJobs(t *testing.T) {
	w := NewWall("")
	w.SetStatePath(filepath.Join(t.TempDir(), "wall.json"))
	c := mediaRenderJobsFor(w)

	older, _, err := c.ensure(farm.MediaRenderJobCreateRequest{
		Identity: "older", RunID: "run-old", Attempts: []int{1}, Mode: "raw", ArtifactKey: "older.mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	newer, _, err := c.ensure(farm.MediaRenderJobCreateRequest{
		Identity: "newer", RunID: "run-new", Attempts: []int{1}, Mode: "broadcast", ArtifactKey: "newer.mp4",
	})
	if err != nil {
		t.Fatal(err)
	}

	c.mu.Lock()
	oldJob := c.state.Jobs[older.ID]
	oldJob.UpdatedAt = 100
	c.state.Jobs[older.ID] = oldJob
	newJob := c.state.Jobs[newer.ID]
	newJob.UpdatedAt = 200
	c.state.Jobs[newer.ID] = newJob
	c.mu.Unlock()

	got := c.list("", "", 1)
	if got.Total != 2 || len(got.Jobs) != 1 || got.Jobs[0].ID != newer.ID {
		t.Fatalf("list = %+v, want newest job with total 2", got)
	}
	if got.States[farm.MediaRenderJobQueued] != 2 {
		t.Fatalf("queued count = %d, want 2", got.States[farm.MediaRenderJobQueued])
	}
	filtered := c.list("run-old", "", 50)
	if filtered.Total != 1 || len(filtered.Jobs) != 1 || filtered.Jobs[0].ID != older.ID {
		t.Fatalf("filtered list = %+v", filtered)
	}
}
