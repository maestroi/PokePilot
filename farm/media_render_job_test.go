package farm

import (
	"testing"
	"time"
)

func TestMediaRenderJobIdentityAndClaimability(t *testing.T) {
	now := time.Unix(100, 0)
	req := MediaRenderJobCreateRequest{
		Identity:    "runs/run-1/replay.mp4",
		RunID:       "run-1",
		Attempts:    []int{1, 2},
		Mode:        "broadcast",
		ArtifactKey: "runs/run-1/replay.mp4",
	}
	a, err := NewMediaRenderJob(req, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewMediaRenderJob(req, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatalf("same identity produced different ids: %q != %q", a.ID, b.ID)
	}
	if !a.Claimable(now) || a.Active() || a.Terminal() {
		t.Fatalf("new job lifecycle wrong: %+v", a)
	}

	a.State = MediaRenderJobRendering
	a.LeaseExpiresAt = now.Add(time.Minute).UnixMilli()
	if a.Claimable(now) {
		t.Fatalf("active unexpired job should not be claimable")
	}
	if !a.Claimable(now.Add(2 * time.Minute)) {
		t.Fatalf("expired active job should be claimable")
	}
}

func TestNewMediaRenderJobRejectsInvalidAttempts(t *testing.T) {
	_, err := NewMediaRenderJob(MediaRenderJobCreateRequest{
		Identity: "x", RunID: "run-1", Mode: "raw", ArtifactKey: "x", Attempts: []int{0},
	}, time.Now())
	if err == nil {
		t.Fatal("expected invalid attempt error")
	}
}
