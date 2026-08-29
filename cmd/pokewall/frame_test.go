package main

// The dashboard frame endpoint: the browser only ever talks to the wall,
// and the wall reaches a specific runner over the swarm network using the
// addresses that runner reported in its heartbeats. These tests pin the
// proxy semantics: only running runs have frames, the exact upstream bytes
// come back, dead addresses fall through to the next, and finished runs
// stop serving frames.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestWallFrameProxy(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 1, 2, 3}

	runner := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/frame.png" {
			res.WriteHeader(http.StatusNotFound)
			return
		}
		res.Header().Set("Content-Type", "image/png")
		res.Write(png) //nolint:errcheck // test server
	}))
	t.Cleanup(runner.Close)

	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	spec := farm.Spec{RunID: "frame-1", Planner: "scripted", Starter: "squirtle", Dest: "pallet"}
	body, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	resp, err := http.Post(srv.URL+"/v1/specs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/specs: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/specs = %d, want 200", resp.StatusCode)
	}

	if code := frameStatus(t, srv.URL, spec.RunID); code != http.StatusNotFound {
		t.Fatalf("/frame before lease = %d, want 404 (not running yet)", code)
	}

	client := farm.NewClient(srv.URL)
	got, err := client.Lease(ctx)
	if err != nil || got == nil {
		t.Fatalf("client.Lease = %v, %v; want the enqueued spec", got, err)
	}

	// Heartbeat reports two worker addresses: the first is dead (nothing
	// listens on loopback port 1), the second is the fake runner. The proxy
	// must fall through to the live one.
	hb := farm.Heartbeat{RunID: spec.RunID, Frame: 100, Map: 0x0c, X: 1, Y: 2}
	hb.WorkerAddrs = []string{"127.0.0.1:1", runner.Listener.Addr().String()}
	if _, err := client.Heartbeat(ctx, hb); err != nil {
		t.Fatalf("client.Heartbeat: %v", err)
	}

	resp, err = http.Get(srv.URL + "/frame?run=" + spec.RunID)
	if err != nil {
		t.Fatalf("GET /frame: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /frame = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store (stale frames mislead)", cc)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if !bytes.Equal(data, png) {
		t.Fatalf("frame = %x, want the upstream bytes %x", data, png)
	}

	if code := frameStatus(t, srv.URL, "no-such-run"); code != http.StatusNotFound {
		t.Errorf("/frame unknown run = %d, want 404", code)
	}

	if err := client.Finish(ctx, farm.FinishReport{RunID: spec.RunID, Reason: "done"}); err != nil {
		t.Fatalf("client.Finish: %v", err)
	}
	// The last live fetch is kept so history cards still have a screen.
	resp, err = http.Get(srv.URL + "/frame?run=" + spec.RunID)
	if err != nil {
		t.Fatalf("GET /frame after finish: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/frame after finish = %d, want 200 (last screen)", resp.StatusCode)
	}
	after, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read frame after finish: %v", err)
	}
	if !bytes.Equal(after, png) {
		t.Fatalf("frame after finish = %x, want the last live bytes %x", after, png)
	}
}

// frameStatus is the status-only variant of a /frame probe.
func frameStatus(t *testing.T, base, runID string) int {
	t.Helper()
	resp, err := http.Get(base + "/frame?run=" + runID)
	if err != nil {
		t.Fatalf("GET /frame?run=%s: %v", runID, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // test probe
	return resp.StatusCode
}
