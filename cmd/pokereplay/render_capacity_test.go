package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestReplayAdmissionQueuesBeyondJobLimit(t *testing.T) {
	s := newReplayServer("http://wall.invalid", "", "", nil)
	s.capacity.ScratchDir = t.TempDir()
	s.capacity.MinScratchBytes = 100
	s.scratchFree = func(string) (uint64, error) { return 1000, nil }
	s.jobSlots = make(chan struct{}, 1)

	release, reason, detail := s.tryAdmitRenderJob("job-1")
	if release == nil || reason != "" || detail != "" {
		t.Fatalf("first admission release=%v reason=%q detail=%q", release != nil, reason, detail)
	}
	defer release()

	second, reason, _ := s.tryAdmitRenderJob("job-2")
	if second != nil {
		second()
		t.Fatal("second job should remain queued while job slot is occupied")
	}
	if reason != "max_jobs" {
		t.Fatalf("queue reason=%q, want max_jobs", reason)
	}
	health := s.replayResourceHealth()
	if got := health["offline_queue_depth"]; got != 1 {
		t.Fatalf("queue depth=%v, want 1", got)
	}
}

func TestReplayAdmissionQueuesOnLowScratch(t *testing.T) {
	s := newReplayServer("http://wall.invalid", "", "", nil)
	s.capacity.ScratchDir = t.TempDir()
	s.capacity.MinScratchBytes = 1000
	s.scratchFree = func(string) (uint64, error) { return 999, nil }
	s.jobSlots = make(chan struct{}, 1)

	release, reason, detail := s.tryAdmitRenderJob("job-low-disk")
	if release != nil {
		release()
		t.Fatal("low scratch unexpectedly admitted render")
	}
	if reason != "low_scratch" || detail == "" {
		t.Fatalf("reason=%q detail=%q", reason, detail)
	}
	if len(s.jobSlots) != 0 {
		t.Fatalf("low scratch consumed a job slot: %d", len(s.jobSlots))
	}
}

func TestPrepareReplayScratchRemovesOnlyOrphanJobDirs(t *testing.T) {
	root := t.TempDir()
	orphan := filepath.Join(root, replayScratchJobPrefix+"old")
	keep := filepath.Join(root, "keep")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "partial.mp4"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareReplayScratch(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan scratch still exists: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unrelated scratch entry removed: %v", err)
	}
}

func TestReplaySegmentDeadlineKillsHungEncoder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake encoder helper is a POSIX shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "hung-stream")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := newReplayServer("http://wall.invalid", "", script, nil)
	ctx, cancel := withReplayTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := s.renderRecordingSegment(ctx, "rom.gb", "run.gbrun", filepath.Join(dir, "out.mp4"))
	if err == nil {
		t.Fatal("hung encoder unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("hung encoder exceeded bounded deadline: %s", elapsed)
	}
	if got := classifyRenderFailure(err, ctx.Err()); got != farm.MediaRenderFailureTimeout {
		t.Fatalf("failure class=%q, want timeout; err=%v ctx=%v", got, err, ctx.Err())
	}
	if got := s.encoderProcesses.Load(); got != 0 {
		t.Fatalf("encoder process count=%d after cancellation", got)
	}
}

func TestReplayActiveCancellationIsImmediate(t *testing.T) {
	s := newReplayServer("http://wall.invalid", "", "", nil)
	cancelled := make(chan struct{}, 1)
	s.registerRenderCancel("job-1", func() { cancelled <- struct{}{} })
	if !s.cancelActiveRender("job-1") {
		t.Fatal("active render was not found")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("active render cancellation did not propagate")
	}
	s.unregisterRenderCancel("job-1")
	if s.cancelActiveRender("job-1") {
		t.Fatal("unregistered render remained cancellable")
	}
}

func TestClassifyRenderFailureSeparatesInvalidAndInfrastructure(t *testing.T) {
	if got := classifyRenderFailure(context.DeadlineExceeded, context.DeadlineExceeded); got != farm.MediaRenderFailureTimeout {
		t.Fatalf("deadline class=%q", got)
	}
	if got := classifyRenderFailure(assertError("recording sha256 mismatch: got a want b"), nil); got != farm.MediaRenderFailureInvalidRequest {
		t.Fatalf("invalid class=%q", got)
	}
	if got := classifyRenderFailure(assertError("temporary S3 unavailable"), nil); got != farm.MediaRenderFailureInfrastructure {
		t.Fatalf("infrastructure class=%q", got)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
