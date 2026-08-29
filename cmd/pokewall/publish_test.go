package main

// The wall's publish mode: on hosts where the swarm ingress network is
// unreachable from the browser, the wall writes the rendered dashboard —
// grid HTML plus each running run's latest frame — to a shared directory,
// and a dumb relay (pokeui) serves it. These tests pin what lands on disk:
// the grid page references the per-run frame route, frames are the exact
// upstream bytes, finished runs stop getting frames, and files are written
// atomically (no temp litter, no partial reads).

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestWallPublish(t *testing.T) {
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
	client := farm.NewClient(srv.URL)
	spec := farm.Spec{RunID: "publish-1", Planner: "scripted", Starter: "squirtle", Dest: "pallet"}
	enqueueViaHTTP(t, srv.URL, spec)
	if got, err := client.Lease(ctx); err != nil || got == nil || got.RunID != spec.RunID {
		t.Fatalf("client.Lease = %v, %v; want the enqueued spec", got, err)
	}

	dir := t.TempDir()
	if err := w.Publish(dir); err != nil {
		t.Fatalf("Publish (not running yet): %v", err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read published index: %v", err)
	}
	if strings.Contains(string(page), "<img") {
		t.Errorf("published grid has a frame <img> before the run is running:\n%s", page)
	}

	hb := farm.Heartbeat{RunID: spec.RunID, Frame: 42, Map: 0x0c, X: 1, Y: 2}
	hb.WorkerAddrs = []string{"127.0.0.1:1", runner.Listener.Addr().String()} // first addr is dead
	if _, err := client.Heartbeat(ctx, hb); err != nil {
		t.Fatalf("client.Heartbeat: %v", err)
	}

	if err := w.Publish(dir); err != nil {
		t.Fatalf("Publish (running): %v", err)
	}
	page, err = os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read published index: %v", err)
	}
	if !strings.Contains(string(page), `<img src="/frame?run=publish-1`) {
		t.Errorf("published grid does not reference the run's frame route:\n%s", page)
	}
	frame, err := os.ReadFile(filepath.Join(dir, "live", "publish-1.png"))
	if err != nil {
		t.Fatalf("read published frame: %v", err)
	}
	if !bytes.Equal(frame, png) {
		t.Errorf("published frame = %x, want the upstream bytes %x", frame, png)
	}
	if leftovers := dirEntries(t, dir); len(leftovers) != 2 || leftovers[0] != "index.html" || leftovers[1] != "live" {
		t.Errorf("publish directory contents = %v, want [index.html live] (no temp litter)", leftovers)
	}

	if err := client.Finish(ctx, farm.FinishReport{RunID: spec.RunID, Reason: "done"}); err != nil {
		t.Fatalf("client.Finish: %v", err)
	}
	before := fileMtime(t, filepath.Join(dir, "live", "publish-1.png"))
	if err := w.Publish(dir); err != nil {
		t.Fatalf("Publish (finished): %v", err)
	}
	page, _ = os.ReadFile(filepath.Join(dir, "index.html"))
	if strings.Contains(string(page), "<img") {
		t.Errorf("published grid still shows a frame after the run finished:\n%s", page)
	}
	if after := fileMtime(t, filepath.Join(dir, "live", "publish-1.png")); !after.Equal(before) {
		t.Errorf("finished run's frame was rewritten (mtime %s -> %s)", before, after)
	}
}

// dirEntries lists a directory's entry names, sorted.
func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func fileMtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime()
}

// enqueueViaHTTP posts a spec the way an operator would, keeping this test
// on the public API rather than internal wall calls.
func enqueueViaHTTP(t *testing.T, base string, spec farm.Spec) {
	t.Helper()
	body, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	resp, err := http.Post(base+"/v1/specs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/specs: %v", err)
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // test probe
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/specs = %d, want 200", resp.StatusCode)
	}
}
