package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

const testRenderStateJSON = `{"schema_version":1,"game":{"id":"pokemon-red","revision":"en-us-rev0"},"clock":{"frame":123},"scene":"overworld","capabilities":["map","player","layers"],"map":{"id":"pallet town","name":"PALLET_TOWN","width":20,"height":18},"player":{"id":"player","kind":"player","position":{"x":5,"y":6},"facing":"up"},"layers":[]}`

func TestWallRenderStateProxy(t *testing.T) {
	runner := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/render-state.json" {
			http.NotFound(res, req)
			return
		}
		res.Header().Set("Content-Type", "application/json")
		io.WriteString(res, testRenderStateJSON) //nolint:errcheck // test server
	}))
	t.Cleanup(runner.Close)

	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	spec := farm.Spec{RunID: "render-1", Planner: "scripted", Starter: "squirtle", Dest: "pallet"}
	body, _ := json.Marshal(spec)
	resp, err := http.Post(srv.URL+"/v1/specs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST spec: %v", err)
	}
	resp.Body.Close()

	client := farm.NewClient(srv.URL)
	if got, err := client.Lease(ctx); err != nil || got == nil {
		t.Fatalf("lease = %v, %v", got, err)
	}
	hb := farm.Heartbeat{RunID: spec.RunID, WorkerAddrs: []string{runner.Listener.Addr().String()}}
	if _, err := client.Heartbeat(ctx, hb); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	resp, err = http.Get(srv.URL + "/render-state?run=" + spec.RunID)
	if err != nil {
		t.Fatalf("GET render state: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET render state = %d, want 200: %s", resp.StatusCode, got)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	if string(got) != testRenderStateJSON {
		t.Fatalf("render state = %q", got)
	}

	if err := client.Finish(ctx, farm.FinishReport{RunID: spec.RunID, Reason: "done"}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if code := renderStateStatus(t, srv.URL, spec.RunID); code != http.StatusNotFound {
		t.Fatalf("render state after finish = %d, want 404", code)
	}
}

func TestWallRenderStateRejectsInvalidUpstreamJSON(t *testing.T) {
	runner := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		io.WriteString(res, `{"schema_version":99}`) //nolint:errcheck // test server
	}))
	t.Cleanup(runner.Close)

	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	spec := farm.Spec{RunID: "bad-render", Planner: "scripted", Starter: "squirtle", Dest: "pallet"}
	body, _ := json.Marshal(spec)
	resp, err := http.Post(srv.URL+"/v1/specs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST spec: %v", err)
	}
	resp.Body.Close()
	client := farm.NewClient(srv.URL)
	if got, err := client.Lease(ctx); err != nil || got == nil {
		t.Fatalf("lease = %v, %v", got, err)
	}
	hb := farm.Heartbeat{RunID: spec.RunID, WorkerAddrs: []string{runner.Listener.Addr().String()}}
	if _, err := client.Heartbeat(ctx, hb); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	if code := renderStateStatus(t, srv.URL, "bad-render"); code != http.StatusBadGateway {
		t.Fatalf("invalid render state = %d, want 502", code)
	}
}

func TestRenderStateFetchCoalescesConcurrentViewers(t *testing.T) {
	var calls atomic.Int32
	runner := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		res.Header().Set("Content-Type", "application/json")
		io.WriteString(res, testRenderStateJSON) //nolint:errcheck // test server
	}))
	t.Cleanup(runner.Close)

	w := NewWall("")
	addr := runner.Listener.Addr().String()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := w.fetchRunnerRenderStateCached(context.Background(), "same-run", []string{addr})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("cached fetch: %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func renderStateStatus(t *testing.T, base, runID string) int {
	t.Helper()
	resp, err := http.Get(base + "/render-state?run=" + runID)
	if err != nil {
		t.Fatalf("GET render-state: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // test probe
	return resp.StatusCode
}
