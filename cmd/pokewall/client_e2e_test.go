package main

// Cross-component smoke test: farm.Client (the runner side) against the
// real Wall handler (the orchestrator side), end to end over httptest. The
// old fake-wall test in cmd/pokepilot only repeated farm/client.go's unit
// tests and could pass while pokewall was incompatible; this one fails if
// either side's route, method, JSON, status, cancellation, or finish
// semantics drift.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestFarmClientRoundTripsAgainstWall(t *testing.T) {
	dumps := t.TempDir()
	w := NewWall(dumps)
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	spec := farm.Spec{
		RunID:     "smoke-1",
		Seed:      42,
		Planner:   "llm",
		Starter:   "bulbasaur",
		Dest:      "pallet",
		FPS:       60,
		MaxRounds: 3,
		MaxFrames: 1000,
	}

	// Enqueue through the public endpoint only — no internal helpers.
	body, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	resp, err := http.Post(srv.URL+"/v1/specs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/specs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/specs = %d, want 200 (%s)", resp.StatusCode, b)
	}

	// Lease through the client and confirm the exact spec came back.
	client := farm.NewClient(srv.URL)
	got, err := client.Lease(ctx)
	if err != nil {
		t.Fatalf("client.Lease: %v", err)
	}
	if got == nil {
		t.Fatal("client.Lease = nil spec, want the enqueued spec")
	}
	wantSpec := spec
	wantSpec.Attempt = 1 // the wall numbers attempts from one
	if *got != wantSpec {
		t.Fatalf("leased spec = %+v, want %+v", *got, wantSpec)
	}

	// Heartbeat: no cancel requested yet.
	hb := farm.Heartbeat{RunID: spec.RunID, Frame: 1200, Map: 0x0c, X: 5, Y: 6, Trace: "route-2"}
	reply, err := client.Heartbeat(ctx, hb)
	if err != nil {
		t.Fatalf("client.Heartbeat: %v", err)
	}
	if reply.Cancel {
		t.Fatal("heartbeat before cancel: cancel = true, want false")
	}

	// Cancel through the public endpoint, then confirm the client sees it.
	resp, err = http.Post(srv.URL+"/v1/runs/"+spec.RunID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST cancel = %d, want 200", resp.StatusCode)
	}
	reply, err = client.Heartbeat(ctx, hb)
	if err != nil {
		t.Fatalf("client.Heartbeat after cancel: %v", err)
	}
	if !reply.Cancel {
		t.Fatal("heartbeat after cancel: cancel = false, want true")
	}

	// Finish through the client with a representative report.
	art := hashedNamed("round-001-frame-0000000100-goto.state", "application/octet-stream", []byte("ckpt"))
	report := farm.FinishReport{
		RunID:         spec.RunID,
		Reason:        "cancelled",
		Detail:        "operator cancel at route 2",
		TraceTail:     []string{"enter route-2", "stop on cancel"},
		SaveState:     []byte("state-bytes"),
		RunnerVersion: "e2e-sha",
		SeedBurn:      0,
		Artifacts:     []farm.Artifact{art},
	}
	if err := client.Finish(ctx, report); err != nil {
		t.Fatalf("client.Finish: %v", err)
	}

	// Wall status: the grid reports the run as done.
	resp, err = http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	grid, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read grid: %v", err)
	}
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(grid), spec.RunID) || !strings.Contains(string(grid), "done") {
		t.Fatalf("grid after finish (status %d): missing run or done status:\n%s", resp.StatusCode, grid)
	}

	// Durable dump: written to the configured directory and decodable.
	dumpPath := filepath.Join(dumps, spec.RunID+".json")
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump %s: %v", dumpPath, err)
	}
	var dumped farm.FinishReport
	if err := json.Unmarshal(data, &dumped); err != nil {
		t.Fatalf("decode dump: %v", err)
	}
	if !reflect.DeepEqual(dumped, report) {
		t.Fatalf("dump = %+v, want %+v", dumped, report)
	}

	// Queue drained: the next lease is (nil, nil), not an error.
	got, err = client.Lease(ctx)
	if err != nil {
		t.Fatalf("client.Lease after finish: %v", err)
	}
	if got != nil {
		t.Fatalf("client.Lease after finish = %+v, want nil (204)", *got)
	}
}
