package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestEndlessEnqueuesSuccessor(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{
		RunID: "loop-1", Planner: "llm", Starter: "squirtle",
		Goal: "Earn the Boulder Badge.", Seed: 7, Endless: true, RandomSeed: true,
	})

	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease: %v %v", spec, err)
	}
	if spec.Goal != "Earn the Boulder Badge." {
		t.Fatalf("leased goal = %q", spec.Goal)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "loop-1", Attempt: spec.Attempt, Reason: "done"}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	got := getDashboard(t, w.Handler())
	if len(got.Runs) != 2 {
		t.Fatalf("after endless settle: %d runs, want 2", len(got.Runs))
	}
	var nextID string
	for _, r := range got.Runs {
		if r.RunID == "loop-1" {
			continue
		}
		nextID = r.RunID
		if r.Status != "queued" || !r.Endless || !r.RandomSeed || r.Goal != "Earn the Boulder Badge." {
			t.Fatalf("successor = %+v", r)
		}
		if r.Seed == 7 {
			t.Fatal("random_seed successor kept the parent seed")
		}
	}
	if nextID == "" {
		t.Fatal("no successor run")
	}
}

func TestEndlessCancelledDoesNotEnqueue(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{
		RunID: "stop-1", Planner: "llm", Starter: "squirtle", Endless: true,
	})
	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease: %v %v", spec, err)
	}
	res, err := http.Post(srv.URL+"/v1/runs/stop-1/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck // test probe
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST cancel = %d, want 200", res.StatusCode)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "stop-1", Attempt: spec.Attempt, Reason: "error", Detail: "cancelled"}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got := getDashboard(t, w.Handler())
	if len(got.Runs) != 1 {
		t.Fatalf("cancelled endless spawned %d runs, want 1", len(got.Runs))
	}
}
