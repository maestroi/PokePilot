package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func newTestServer(t *testing.T, dumpsDir string) *httptest.Server {
	t.Helper()
	wall := NewWall(dumpsDir)
	srv := httptest.NewServer(wall.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, url string, v any) *http.Response {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, buf.String()
}

func spec(runID string) farm.Spec {
	return farm.Spec{RunID: runID, Seed: 42, Planner: "scripted", Starter: "charmander", Dest: "pallet", FPS: 60, MaxRounds: 3, MaxFrames: 1000}
}

func TestSpecsEnqueue(t *testing.T) {
	srv := newTestServer(t, "")
	// Accepts a farm.Spec and enqueues it.
	if resp := postJSON(t, srv.URL+"/v1/specs", spec("run-1")); resp.StatusCode != http.StatusOK {
		t.Fatalf("spec run-1: status %d", resp.StatusCode)
	}
	// Rejects a missing run_id.
	if resp := postJSON(t, srv.URL+"/v1/specs", farm.Spec{Planner: "scripted"}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing run_id: status %d, want 400", resp.StatusCode)
	}
	// Rejects a duplicate active run_id.
	if resp := postJSON(t, srv.URL+"/v1/specs", spec("run-1")); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate active run_id: status %d, want 409", resp.StatusCode)
	}
}

func TestLeaseOldestOnceThenEmpty(t *testing.T) {
	srv := newTestServer(t, "")
	for _, id := range []string{"a", "b"} {
		if resp := postJSON(t, srv.URL+"/v1/specs", spec(id)); resp.StatusCode != http.StatusOK {
			t.Fatalf("spec %s: status %d", id, resp.StatusCode)
		}
	}
	// Oldest first.
	resp := postJSON(t, srv.URL+"/v1/lease", struct{}{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lease 1: status %d", resp.StatusCode)
	}
	var got farm.Spec
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode lease: %v", err)
	}
	if got.RunID != "a" {
		t.Fatalf("lease 1 run_id = %q, want a", got.RunID)
	}
	// Second lease returns the next spec.
	resp = postJSON(t, srv.URL+"/v1/lease", struct{}{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lease 2: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode lease 2: %v", err)
	}
	if got.RunID != "b" {
		t.Fatalf("lease 2 run_id = %q, want b", got.RunID)
	}
	// Then 204.
	resp = postJSON(t, srv.URL+"/v1/lease", struct{}{})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("lease 3: status %d, want 204", resp.StatusCode)
	}
}

func TestHeartbeat(t *testing.T) {
	srv := newTestServer(t, "")
	postJSON(t, srv.URL+"/v1/specs", spec("h1"))
	postJSON(t, srv.URL+"/v1/lease", struct{}{})

	hb := farm.Heartbeat{RunID: "h1", Frame: 7, Map: 0x0c, X: 3, Y: 4, Trace: "stepped north", StopSoFar: ""}
	resp := postJSON(t, srv.URL+"/v1/runs/h1/heartbeat", hb)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat: status %d", resp.StatusCode)
	}
	var reply farm.HeartbeatReply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.Cancel {
		t.Fatal("cancel = true, want false")
	}

	// URL/body run ID mismatch is rejected.
	resp = postJSON(t, srv.URL+"/v1/runs/other/heartbeat", hb)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("mismatched id: status %d, want 400", resp.StatusCode)
	}
	// Unknown run ID is a 404.
	resp = postJSON(t, srv.URL+"/v1/runs/nope/heartbeat", farm.Heartbeat{RunID: "nope"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id: status %d, want 404", resp.StatusCode)
	}
}

func TestCancel(t *testing.T) {
	srv := newTestServer(t, "")
	postJSON(t, srv.URL+"/v1/specs", spec("c1"))
	postJSON(t, srv.URL+"/v1/lease", struct{}{})

	if resp := postJSON(t, srv.URL+"/v1/runs/c1/cancel", struct{}{}); resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel: status %d", resp.StatusCode)
	}
	// A later heartbeat returns cancel=true.
	resp := postJSON(t, srv.URL+"/v1/runs/c1/heartbeat", farm.Heartbeat{RunID: "c1"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat after cancel: status %d", resp.StatusCode)
	}
	var reply farm.HeartbeatReply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if !reply.Cancel {
		t.Fatal("cancel = false after /cancel, want true")
	}
	// Unknown IDs return 404.
	if resp := postJSON(t, srv.URL+"/v1/runs/nope/cancel", struct{}{}); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cancel unknown: status %d, want 404", resp.StatusCode)
	}
}

func TestFinish(t *testing.T) {
	srv := newTestServer(t, "")
	postJSON(t, srv.URL+"/v1/specs", spec("f1"))
	postJSON(t, srv.URL+"/v1/lease", struct{}{})

	report := farm.FinishReport{RunID: "f1", Reason: "stuck", Detail: "no progress 3 rounds", TraceTail: []string{"a", "b"}}
	resp := postJSON(t, srv.URL+"/v1/runs/f1/finish", report)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("finish: status %d", resp.StatusCode)
	}
	// Duplicate identical finish is idempotent.
	resp = postJSON(t, srv.URL+"/v1/runs/f1/finish", report)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("identical duplicate finish: status %d, want 200", resp.StatusCode)
	}
	// A conflicting duplicate is rejected.
	conflict := report
	conflict.Detail = "something else"
	resp = postJSON(t, srv.URL+"/v1/runs/f1/finish", conflict)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("conflicting duplicate finish: status %d, want 409", resp.StatusCode)
	}
	// URL/body mismatch.
	resp = postJSON(t, srv.URL+"/v1/runs/other/finish", report)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("mismatched id: status %d, want 400", resp.StatusCode)
	}
	// Unknown run ID.
	resp = postJSON(t, srv.URL+"/v1/runs/nope/finish", farm.FinishReport{RunID: "nope"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id: status %d, want 404", resp.StatusCode)
	}
}

func TestDurableDump(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServer(t, dir)
	postJSON(t, srv.URL+"/v1/specs", spec("d1"))
	postJSON(t, srv.URL+"/v1/lease", struct{}{})

	report := farm.FinishReport{RunID: "d1", Reason: "done", Detail: "arrived", TraceTail: []string{"x"}}
	if resp := postJSON(t, srv.URL+"/v1/runs/d1/finish", report); resp.StatusCode != http.StatusOK {
		t.Fatalf("finish: status %d", resp.StatusCode)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dumps dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dump files = %d, want 1", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	var saved farm.FinishReport
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode dump: %v", err)
	}
	if saved.RunID != "d1" || saved.Reason != "done" {
		t.Fatalf("decoded dump = %+v", saved)
	}
}

func TestDurableDumpNoPathTraversal(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServer(t, dir)
	evil := spec("../../etc/evil")
	if resp := postJSON(t, srv.URL+"/v1/specs", evil); resp.StatusCode != http.StatusOK {
		t.Fatalf("spec: status %d", resp.StatusCode)
	}
	postJSON(t, srv.URL+"/v1/lease", struct{}{})
	report := farm.FinishReport{RunID: "../../etc/evil", Reason: "stuck", Detail: "x"}
	if resp := postJSON(t, srv.URL+"/v1/runs/..%2F..%2Fetc%2Fevil/finish", report); resp.StatusCode != http.StatusOK {
		t.Fatalf("finish: status %d (%s)", resp.StatusCode, resp.Body)
	}
	// Nothing may have escaped the dumps directory.
	if _, err := os.Stat(filepath.Join(dir, "..", "etc")); !os.IsNotExist(err) {
		t.Fatal("path traversal wrote outside the dumps dir")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dumps dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dump files = %d, want 1", len(entries))
	}
	for _, e := range entries {
		if strings.ContainsAny(e.Name(), "/\\") || filepath.Clean(e.Name()) != e.Name() {
			t.Fatalf("unsafe dump filename %q", e.Name())
		}
	}
}

func TestFinishDumpRetryAfterFailure(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServer(t, dir)
	postJSON(t, srv.URL+"/v1/specs", spec("r1"))
	postJSON(t, srv.URL+"/v1/lease", struct{}{})

	report := farm.FinishReport{RunID: "r1", Reason: "stuck", Detail: "no progress"}
	// Force the first durable write to fail: occupy the dump path with a
	// directory so the file write cannot succeed.
	dumpPath := filepath.Join(dir, safeDumpName("r1"))
	if err := os.Mkdir(dumpPath, 0o755); err != nil {
		t.Fatalf("seed failure: %v", err)
	}
	resp := postJSON(t, srv.URL+"/v1/runs/r1/finish", report)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("first finish succeeded despite unwritable dump path")
	}
	// Restore writability and retry the identical FinishReport.
	if err := os.Remove(dumpPath); err != nil {
		t.Fatalf("restore: %v", err)
	}
	resp = postJSON(t, srv.URL+"/v1/runs/r1/finish", report)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retry finish: status %d, want 200", resp.StatusCode)
	}
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump after retry: %v", err)
	}
	var saved farm.FinishReport
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode dump after retry: %v", err)
	}
	if saved.RunID != "r1" || saved.Reason != "stuck" || saved.Detail != report.Detail {
		t.Fatalf("decoded dump = %+v", saved)
	}
}

func TestGridEscapesOperatorStrings(t *testing.T) {
	srv := newTestServer(t, "")
	evil := spec("<script>alert(1)</script>")
	if resp := postJSON(t, srv.URL+"/v1/specs", evil); resp.StatusCode != http.StatusOK {
		t.Fatalf("spec: status %d", resp.StatusCode)
	}
	postJSON(t, srv.URL+"/v1/lease", struct{}{})
	postJSON(t, srv.URL+"/v1/runs/"+url.PathEscape("<script>alert(1)</script>")+"/heartbeat", farm.Heartbeat{
		RunID: "<script>alert(1)</script>",
		Trace: "b < i && i > a",
	})
	code, body := get(t, srv.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("GET /: status %d", code)
	}
	if strings.Contains(body, "<script>") {
		t.Fatal("grid contains unescaped operator input")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("grid does not contain escaped run id")
	}
	if !strings.Contains(body, "b &lt; i &amp;&amp; i &gt; a") {
		t.Fatal("grid does not contain escaped trace")
	}
}

// TestGridConcurrentMutations is the race regression: GET / must render a
// snapshot of plain values taken under the lock, never live tile pointers
// read after unlock. Under -race, rendering []*Tile after unlock while
// heartbeat/cancel/finish mutate tiles fails here.
func TestGridConcurrentMutations(t *testing.T) {
	srv := newTestServer(t, "")
	const n = 8
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("g%d", i)
		if resp := postJSON(t, srv.URL+"/v1/specs", spec(id)); resp.StatusCode != http.StatusOK {
			t.Fatalf("spec %s: status %d", id, resp.StatusCode)
		}
	}
	for i := 0; i < n; i++ {
		if resp := postJSON(t, srv.URL+"/v1/lease", struct{}{}); resp.StatusCode != http.StatusOK {
			t.Fatalf("lease %d: status %d", i, resp.StatusCode)
		}
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				id := fmt.Sprintf("g%d", (w+i)%n)
				switch i % 4 {
				case 0:
					resp, err := http.Post(srv.URL+"/v1/runs/"+id+"/heartbeat", "application/json",
						strings.NewReader(fmt.Sprintf(`{"run_id":%q,"frame":%d}`, id, i)))
					if err == nil {
						resp.Body.Close()
					}
				case 1:
					resp, err := http.Post(srv.URL+"/v1/runs/"+id+"/cancel", "application/json", strings.NewReader("{}"))
					if err == nil {
						resp.Body.Close()
					}
				case 2:
					resp, err := http.Post(srv.URL+"/v1/runs/"+id+"/finish", "application/json",
						strings.NewReader(fmt.Sprintf(`{"run_id":%q,"reason":"stuck","detail":"round %d"}`, id, i)))
					if err == nil {
						resp.Body.Close()
					}
				default:
					resp, err := http.Get(srv.URL + "/")
					if err == nil {
						resp.Body.Close()
					}
				}
			}
		}(w)
	}
	// Give the workers a real window of overlap.
	for i := 0; i < 300; i++ {
		if code, _ := get(t, srv.URL+"/"); code != http.StatusOK {
			t.Fatalf("GET /: status %d", code)
		}
	}
	close(stop)
	wg.Wait()
}
