package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSpectatorServesReadOnlySanitizedSurface(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 1, 2, 3}
	var upstreamCalls atomic.Int32
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		upstreamCalls.Add(1)
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/dashboard":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{
				"now": 123,
				"wall_version": "secret-wall-sha",
				"workers": [{"addr":"10.0.0.9:8099","version":"secret-runner-sha"}],
				"runs": [{
					"run_id":"run-1","status":"running","planner":"llm","starter":"bulbasaur","dest":"brock","goal":"Get the Boulder Badge",
					"seed":999,"queued_at":100,"frame":456,"map":12,"x":15,"y":9,
					"trace":"private trace","question":"private question","decision":"Travel to Pewter City","raw":"private raw exchange","stop_so_far":"No badge yet",
					"stats":{"round":3,"rounds_left":7,"calls":4,"rounds":3,"rejected":1,"repeats":1,"last_seconds":2.5,"avg_seconds":2.0,"model":"private-model","backend":"fallback","prompt_tokens":5000},
					"player":{"money":1200,"badges":["Boulder"],"party":[{"name":"BULBASAUR","level":12,"hp":25,"max_hp":31}]},
					"attempts":1,"reason":"","detail":"private failure detail","issue":{"number":42}
				}]
			}`))
		case req.Method == http.MethodGet && req.URL.Path == "/frame" && req.URL.Query().Get("run") == "run-1":
			res.Header().Set("Content-Type", "image/png")
			res.Write(png)
		default:
			res.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(wall.Close)

	ui := httptest.NewServer(spectatorHandler(wall.URL))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/")
	if err != nil {
		t.Fatalf("GET spectator index: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", res.StatusCode)
	}
	for _, want := range []string{"PokéPilot Spectator Mode", "Read only", "/watch.js"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("spectator index missing %q", want)
		}
	}
	if got := res.Header.Get("Content-Security-Policy"); !strings.Contains(got, "form-action 'none'") {
		t.Errorf("spectator CSP = %q, want form-action none", got)
	}
	if got := res.Header.Get("Content-Security-Policy"); !strings.Contains(got, "img-src") || !strings.Contains(got, "blob:") {
		t.Errorf("spectator CSP = %q, want img-src to allow blob: (20 fps pump paints object URLs)", got)
	}

	res, err = http.Get(ui.URL + "/watch.js")
	if err != nil {
		t.Fatalf("GET watch.js: %v", err)
	}
	js, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /watch.js = %d, want 200", res.StatusCode)
	}
	if !bytes.Contains(js, []byte("/v1/watch")) {
		t.Errorf("watch.js missing public snapshot route")
	}
	for _, forbidden := range []string{"/v1/specs", "/cancel", "/mcp", "/v1/triage"} {
		if bytes.Contains(js, []byte(forbidden)) {
			t.Errorf("watch.js unexpectedly contains control route %q", forbidden)
		}
	}

	res, err = http.Get(ui.URL + "/v1/watch")
	if err != nil {
		t.Fatalf("GET watch snapshot: %v", err)
	}
	watchBody, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/watch = %d, want 200: %s", res.StatusCode, watchBody)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("watch Cache-Control = %q, want no-store", cc)
	}
	for _, secret := range []string{"secret-wall-sha", "10.0.0.9", "secret-runner-sha", "999", "private trace", "private question", "private raw exchange", "private failure detail", "private-model", "fallback", "prompt_tokens", "\"issue\"", "\"workers\""} {
		if bytes.Contains(watchBody, []byte(secret)) {
			t.Errorf("public snapshot leaked %q: %s", secret, watchBody)
		}
	}
	for _, want := range []string{"run-1", "bulbasaur", "Get the Boulder Badge", "Travel to Pewter City", "BULBASAUR", "Boulder"} {
		if !bytes.Contains(watchBody, []byte(want)) {
			t.Errorf("public snapshot missing %q: %s", want, watchBody)
		}
	}
	var decoded spectatorDashboard
	if err := json.Unmarshal(watchBody, &decoded); err != nil {
		t.Fatalf("decode public snapshot: %v", err)
	}
	if len(decoded.Runs) != 1 || decoded.Runs[0].Stats == nil || decoded.Runs[0].Stats.Round != 3 {
		t.Fatalf("decoded snapshot = %+v", decoded)
	}

	beforeBlocked := upstreamCalls.Load()
	for _, probe := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/dashboard"},
		{http.MethodGet, "/v1/triage"},
		{http.MethodGet, "/mcp"},
		{http.MethodPost, "/v1/specs"},
		{http.MethodPost, "/v1/runs/run-1/cancel"},
		{http.MethodDelete, "/v1/runs/run-1"},
	} {
		req, err := http.NewRequest(probe.method, ui.URL+probe.path, nil)
		if err != nil {
			t.Fatalf("new %s %s: %v", probe.method, probe.path, err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", probe.method, probe.path, err)
		}
		io.Copy(io.Discard, res.Body) //nolint:errcheck // test probe
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", probe.method, probe.path, res.StatusCode)
		}
	}
	if got := upstreamCalls.Load(); got != beforeBlocked {
		t.Fatalf("blocked spectator routes reached upstream: calls %d -> %d", beforeBlocked, got)
	}

	res, err = http.Get(ui.URL + "/frame?run=run-1")
	if err != nil {
		t.Fatalf("GET spectator frame: %v", err)
	}
	frame, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET spectator frame = %d, want 200", res.StatusCode)
	}
	if !bytes.Equal(frame, png) {
		t.Errorf("spectator frame = %x, want %x", frame, png)
	}
}

// The public page used to assign <img src="/frame?...&t="> every 750ms.
// That is ~1.3 fps — a slideshow next to the operator's 20 fps blob pump.
// The PNG is ~2KB, so the spectator uses the same 50ms sequential fetch.
func TestSpectatorFramePumpIsTwentyFPS(t *testing.T) {
	src := string(watchJS)
	if strings.Contains(src, "setInterval(refreshFrame") {
		t.Fatal("watch.js still interval-polls the frame; that is a 750ms slideshow")
	}
	m := regexp.MustCompile(`const frameMs = (\d+)`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("watch.js frame pump has no frameMs; unbounded fetch+decode burns the tab")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("frameMs: %v", err)
	}
	if n != 50 {
		t.Fatalf("frameMs = %d, want 50 (20 fps; 750 was a slideshow, 0 burns the tab)", n)
	}
	for _, want := range []string{
		`fetch("/frame?run=`,
		`cache: "no-store"`,
		`createObjectURL`,
		`revokeObjectURL`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("watch.js missing %q", want)
		}
	}
}

func TestSpectatorHistoryIsBoundedButKeepsActiveRuns(t *testing.T) {
	runs := make([]spectatorRun, 0, spectatorHistoryLimit+3)
	runs = append(runs, spectatorRun{RunID: "active", Status: "running"})
	for i := 0; i < spectatorHistoryLimit+2; i++ {
		runs = append(runs, spectatorRun{RunID: string(rune('a' + i)), Status: "done"})
	}
	got := spectatorRuns(runs)
	if len(got) != spectatorHistoryLimit+1 {
		t.Fatalf("spectatorRuns len = %d, want %d", len(got), spectatorHistoryLimit+1)
	}
	if got[0].RunID != "active" {
		t.Fatalf("spectatorRuns dropped/reordered active run: %+v", got)
	}
	if got[1].RunID != "c" {
		t.Fatalf("spectatorRuns kept wrong history tail: first done = %q, want c", got[1].RunID)
	}
}
