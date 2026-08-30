package main

// The wall's memory must survive its own restarts: a rolled or crashed wall
// that forgets active runs leaves the grid blank while the games keep
// playing, and a run whose runner truly died must not sit "running" forever.
// These tests pin both halves: state (tiles + queue) persists across a
// restart with live runs resuming seamlessly, and the reaper declares
// stale runs lost (re-queueing them while attempts remain) without
// touching queued ones.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestWallStateSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "wall-state.json")

	// First life: enqueue two runs, lease the first, heartbeat it.
	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())

	ctx := context.Background()
	for _, id := range []string{"first", "second"} {
		enqueueViaHTTP(t, srv1.URL, farm.Spec{RunID: id, Planner: "scripted", Starter: "squirtle", Dest: "pallet"})
	}
	client := farm.NewClient(srv1.URL)
	if got, err := client.Lease(ctx); err != nil || got == nil || got.RunID != "first" {
		t.Fatalf("client.Lease = %v, %v; want first", got, err)
	}
	hb := farm.Heartbeat{RunID: "first", Frame: 7, Map: 0x0c, X: 3, Y: 4,
		Question: "1: go to pallet town", Decision: "go to pallet town"}
	hb.WorkerAddrs = []string{"10.0.0.5:8099"}
	if _, err := client.Heartbeat(ctx, hb); err != nil {
		t.Fatalf("client.Heartbeat: %v", err)
	}
	srv1.Close()

	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not written: %v", err)
	}

	// Second life: a fresh Wall pointed at the same state file.
	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()

	page := doGet(t, srv2.URL+"/")
	for _, want := range []string{"<td>first</td>", "<td>second</td>", "<td>running</td>", "<td>queued</td>"} {
		if !strings.Contains(page, want) {
			t.Errorf("restored grid missing %s:\n%s", want, page)
		}
	}

	dash := doGet(t, srv2.URL+"/v1/dashboard")
	if !strings.Contains(dash, `"question":"1: go to pallet town"`) || !strings.Contains(dash, `"decision":"go to pallet town"`) {
		t.Errorf("restored dashboard dropped the plan:\n%s", dash)
	}

	// The live run's heartbeats continue seamlessly (no 404 "unknown run").
	client2 := farm.NewClient(srv2.URL)
	hb.Frame = 8
	if _, err := client2.Heartbeat(ctx, hb); err != nil {
		t.Fatalf("client.Heartbeat after restart: %v", err)
	}

	// The still-queued run is leased out again, oldest first.
	if got, err := client2.Lease(ctx); err != nil || got == nil || got.RunID != "second" {
		t.Fatalf("client.Lease after restart = %v, %v; want second", got, err)
	}
}

func TestWallReapsStaleRuns(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()

	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "stale-run", Planner: "scripted", Starter: "squirtle", Dest: "pallet"})
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "queued-run", Planner: "scripted", Starter: "squirtle", Dest: "pallet"})

	if got, err := client.Lease(ctx); err != nil || got == nil || got.RunID != "stale-run" {
		t.Fatalf("client.Lease = %v, %v; want stale-run", got, err)
	}
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: "stale-run", Frame: 1}); err != nil {
		t.Fatalf("client.Heartbeat: %v", err)
	}

	// Fresh: nothing is reaped.
	if reaped := w.reapStale(time.Now()); len(reaped) != 0 {
		t.Fatalf("reapStale (fresh) = %v, want none", reaped)
	}

	// Past the expiry with no further heartbeats: the running run is lost,
	// and because attempts remain the wall re-queues it with a fresh seed;
	// the queued one is never reaped.
	reaped := w.reapStale(time.Now().Add(w.staleAfter + time.Second))
	if len(reaped) != 1 || reaped[0] != "stale-run" {
		t.Fatalf("reapStale (stale) = %v, want [stale-run]", reaped)
	}

	page := doGet(t, srv.URL+"/")
	for _, want := range []string{"<td>queued</td>", "attempt 1 failed: no heartbeat for 31s"} {
		if !strings.Contains(page, want) {
			t.Errorf("grid after reap missing %s:\n%s", want, page)
		}
	}

	// A late finish from the zombie runner (its attempt 1) is a conflict,
	// not a clobber: the wall has already moved this run to attempt 2.
	report := `{"run_id":"stale-run","reason":"done","attempt":1}`
	resp, err := http.Post(srv.URL+"/v1/runs/stale-run/finish", "application/json", strings.NewReader(report))
	if err != nil {
		t.Fatalf("POST finish: %v", err)
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // test probe
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("late finish = %d, want 409 (the wall already ruled this run lost)", resp.StatusCode)
	}
}

// doGet fetches a page and returns its body as a string.
func doGet(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET %s = %d, want 200", url, res.StatusCode)
	}
	return string(body)
}
