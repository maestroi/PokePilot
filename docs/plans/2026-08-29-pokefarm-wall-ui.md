# Pokefarm Wall Operator UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the pokefarm wall's HTML table with a Go-embedded operator console (live cards, workers, history, enqueue, cancel) served by `pokeui` as an allowlisted reverse proxy to the wall.

**Architecture:** The wall grows `GET /v1/dashboard` JSON from the same tile/worker snapshot the debug table already renders. `pokeui` embeds one HTML/CSS/JS page at `GET /` and proxies only `/v1/dashboard`, `/v1/specs`, `/v1/runs/{id}/cancel`, and `/frame` to `-wall`. The farm stack adds a `ui` service with host-mode publish so the browser reaches pokeui without swarm ingress, and pokeui reaches `http://wall:8080` on the overlay. Publisher/frames-dir are dropped from the stack command; publisher code and tests stay.

**Tech Stack:** Go 1.26 stdlib (`net/http`, `encoding/json`, `html/template` on the wall debug page only, `go:embed` on pokeui). No Vue, no Node, no new module. Spec: `docs/plans/2026-08-29-pokefarm-wall-ui-design.md`.

**If run by agent-runner:** do not commit (AGENTS.md). Skip every Commit step.

---

### Task 1: Wall `GET /v1/dashboard` JSON

**Files:**
- Create: `cmd/pokewall/dashboard_test.go`
- Modify: `cmd/pokewall/wall.go` (`tileRow`, `workerRow`, `Handler`, extract snapshot, new `handleDashboard`)

**Step 1: Write the failing test**

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

type dashboardJSON struct {
	Now     int64 `json:"now"`
	Runs    []struct {
		RunID     string `json:"run_id"`
		Status    string `json:"status"`
		Planner   string `json:"planner"`
		Starter   string `json:"starter"`
		Dest      string `json:"dest"`
		Seed      int64  `json:"seed"`
		FPS       int    `json:"fps"`
		MaxRounds int    `json:"max_rounds"`
		MaxFrames int    `json:"max_frames"`
		Attempts  int    `json:"attempts"`
		Frame     uint64 `json:"frame"`
		Map       uint8  `json:"map"`
		X         uint8  `json:"x"`
		Y         uint8  `json:"y"`
		Trace     string `json:"trace"`
		StopSoFar string `json:"stop_so_far"`
		Reason    string `json:"reason"`
		Detail    string `json:"detail"`
	} `json:"runs"`
	Workers []struct {
		Addr    string `json:"addr"`
		RunID   string `json:"run_id"`
		SeenAgo string `json:"seen_ago"`
	} `json:"workers"`
}

func getDashboard(t *testing.T, h http.Handler) dashboardJSON {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/dashboard = %d, want 200: %s", res.Code, res.Body.String())
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got dashboardJSON
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode dashboard: %v\n%s", err, res.Body.String())
	}
	if got.Now == 0 {
		t.Error("now is 0, want a unix timestamp")
	}
	return got
}

func TestDashboardJSON(t *testing.T) {
	wall := NewWall("")
	h := wall.Handler()

	got := getDashboard(t, h)
	if len(got.Runs) != 0 || len(got.Workers) != 0 {
		t.Fatalf("empty wall: runs=%d workers=%d, want 0/0", len(got.Runs), len(got.Workers))
	}

	specBody, _ := json.Marshal(farm.Spec{
		RunID: "dash-1", Seed: 42, Planner: "scripted", Starter: "charmander",
		Dest: "pallet", FPS: 60, MaxRounds: 3, MaxFrames: 1000,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/specs", bytes.NewReader(specBody))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/specs = %d", res.Code)
	}

	got = getDashboard(t, h)
	if len(got.Runs) != 1 {
		t.Fatalf("queued runs = %d, want 1", len(got.Runs))
	}
	r := got.Runs[0]
	if r.RunID != "dash-1" || r.Status != "queued" || r.Planner != "scripted" || r.Starter != "charmander" || r.Dest != "pallet" || r.Seed != 42 || r.FPS != 60 || r.MaxRounds != 3 || r.MaxFrames != 1000 {
		t.Fatalf("queued run = %+v", r)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/lease", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/lease = %d", res.Code)
	}
	hbBody, _ := json.Marshal(farm.Heartbeat{
		RunID: "dash-1", Frame: 99, Map: 0x0c, X: 5, Y: 6,
		Trace: "stepped north", StopSoFar: "ok",
		WorkerAddrs: []string{"10.0.1.9:8099"},
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/dash-1/heartbeat", bytes.NewReader(hbBody))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("heartbeat = %d", res.Code)
	}

	got = getDashboard(t, h)
	r = got.Runs[0]
	if r.Status != "running" || r.Frame != 99 || r.Map != 0x0c || r.X != 5 || r.Y != 6 || r.Trace != "stepped north" || r.StopSoFar != "ok" {
		t.Fatalf("running run = %+v", r)
	}
	if len(got.Workers) != 1 || got.Workers[0].Addr != "10.0.1.9:8099" || got.Workers[0].RunID != "dash-1" {
		t.Fatalf("workers = %+v", got.Workers)
	}

	finBody, _ := json.Marshal(farm.FinishReport{RunID: "dash-1", Reason: "done", Detail: "arrived"})
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/dash-1/finish", bytes.NewReader(finBody))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("finish = %d", res.Code)
	}
	got = getDashboard(t, h)
	r = got.Runs[0]
	if r.Status != "done" || r.Reason != "done" || r.Detail != "arrived" {
		t.Fatalf("finished run = %+v", r)
	}
	if r.Trace != "stepped north" {
		t.Errorf("finished run dropped last trace: %+v", r)
	}
}
```

Add `"bytes"` to the import list.

**Step 2: Run it to verify RED**

```bash
go test ./cmd/pokewall -run '^TestDashboardJSON$' -count=1 -v
```

Expected: FAIL (404 or mux does not know `/v1/dashboard`).

**Step 3: Implement the snapshot JSON**

Add json tags to `tileRow` and `workerRow` (snake_case, matching `farm`):

```go
type tileRow struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	Planner   string `json:"planner"`
	Starter   string `json:"starter"`
	Dest      string `json:"dest"`
	Seed      int64  `json:"seed"`
	FPS       int    `json:"fps"`
	MaxRounds int    `json:"max_rounds"`
	MaxFrames int    `json:"max_frames"`
	Frame     uint64 `json:"frame"`
	Map       uint8  `json:"map"`
	X         uint8  `json:"x"`
	Y         uint8  `json:"y"`
	Trace     string `json:"trace"`
	StopSoFar string `json:"stop_so_far"`
	Attempts  int    `json:"attempts"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail"`
}

type workerRow struct {
	Addr    string `json:"addr"`
	RunID   string `json:"run_id"`
	SeenAgo string `json:"seen_ago"`
}

type dashboardView struct {
	Now     int64       `json:"now"`
	Runs    []tileRow   `json:"runs"`
	Workers []workerRow `json:"workers"`
}
```

Extract the lock-and-copy currently inside `renderGrid` into `snapshot()` that returns `dashboardView`. `renderGrid` executes the template from that view (field names `Rows`/`Workers`/`Now` stay on a thin wrapper if the template still uses `.Rows` — map `Runs` → `Rows` in that wrapper so the debug HTML does not change).

Register `mux.HandleFunc("GET /v1/dashboard", w.handleDashboard)` next to the other `/v1` routes.

```go
func (w *Wall) handleDashboard(res http.ResponseWriter, req *http.Request) {
	writeJSON(res, http.StatusOK, w.snapshot())
}
```

Do not put `worker_addrs` in the JSON. `writeJSON` already sets `Content-Type: application/json`.

**Step 4: Run the test to verify GREEN**

```bash
go test ./cmd/pokewall -run '^TestDashboardJSON$|^TestWallWorkerPresence$|^TestWallPublish$' -count=1
```

Expected: PASS. Worker presence still reads the HTML table at `GET /`.

**Step 5: Commit** (skip if agent-runner)

```bash
git add cmd/pokewall/dashboard_test.go cmd/pokewall/wall.go
git commit -m "$(cat <<'EOF'
Add GET /v1/dashboard JSON for the operator console.

EOF
)"
```

---

### Task 2: `pokeui` allowlisted reverse proxy

**Files:**
- Modify: `cmd/pokeui/main.go`
- Modify: `cmd/pokeui/relay_test.go` (replace the file-server tests)

The existing tests pin a directory file server. That contract is gone. Replace them.

**Step 1: Write the failing proxy tests**

Replace `cmd/pokeui/relay_test.go` with:

```go
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPokeuiProxiesAllowlistedRoutes(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 1, 2, 3}
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/dashboard":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"now":1,"runs":[],"workers":[]}`))
		case req.Method == http.MethodPost && req.URL.Path == "/v1/specs":
			body, _ := io.ReadAll(req.Body)
			if !bytes.Contains(body, []byte(`"run_id"`)) {
				res.WriteHeader(http.StatusBadRequest)
				return
			}
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"status":"queued"}`))
		case req.Method == http.MethodPost && req.URL.Path == "/v1/runs/r1/cancel":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"cancel":true}`))
		case req.Method == http.MethodGet && req.URL.Path == "/frame" && req.URL.Query().Get("run") == "demo/1":
			res.Header().Set("Content-Type", "image/png")
			res.Write(png)
		case req.URL.Path == "/v1/lease":
			res.WriteHeader(http.StatusOK)
			res.Write([]byte(`should-not-be-reached`))
		default:
			res.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(wall.Close)

	ui := httptest.NewServer(handler(wall.URL))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/v1/dashboard")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET /v1/dashboard = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("dashboard Content-Type = %q", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("dashboard Cache-Control = %q, want no-store", cc)
	}
	if !bytes.Contains(body, []byte(`"runs"`)) {
		t.Errorf("dashboard body = %s", body)
	}

	spec, _ := json.Marshal(map[string]string{"run_id": "r1"})
	res, err = http.Post(ui.URL+"/v1/specs", "application/json", bytes.NewReader(spec))
	if err != nil {
		t.Fatalf("POST specs: %v", err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("POST /v1/specs = %d, want 200", res.StatusCode)
	}

	res, err = http.Post(ui.URL+"/v1/runs/r1/cancel", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("POST cancel = %d, want 200", res.StatusCode)
	}

	res, err = http.Get(ui.URL + "/frame?run=demo/1")
	if err != nil {
		t.Fatalf("GET frame: %v", err)
	}
	frame, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET /frame = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("frame Content-Type = %q", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("frame Cache-Control = %q, want no-store", cc)
	}
	if !bytes.Equal(frame, png) {
		t.Errorf("frame = %x, want %x", frame, png)
	}

	res, err = http.Post(ui.URL+"/v1/lease", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST lease: %v", err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("POST /v1/lease = %d, want 404 (not allowlisted)", res.StatusCode)
	}
}

func TestPokeuiWallUnreachable(t *testing.T) {
	ui := httptest.NewServer(handler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)
	res, err := http.Get(ui.URL + "/v1/dashboard")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("unreachable wall = %d, want 502", res.StatusCode)
	}
	if !bytes.Contains(body, []byte("wall unreachable")) {
		t.Errorf("502 body = %s, want wall unreachable", body)
	}
}

func TestPokeuiServesIndex(t *testing.T) {
	ui := httptest.NewServer(handler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)
	res, err := http.Get(ui.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET / = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if !bytes.Contains(body, []byte("pokefarm")) {
		t.Errorf("index missing pokefarm: %s", body)
	}
}
```

For this step only, `TestPokeuiServesIndex` may fail until Task 3 embeds the page. Either:

- land a 20-line stub `ui/index.html` containing the word `pokefarm` in this task, or
- skip `TestPokeuiServesIndex` until Task 3.

Prefer the stub so Task 2 is green on its own.

**Step 2: Run tests to verify RED**

```bash
go test ./cmd/pokeui -count=1 -v
```

Expected: compile failure (`handler` still takes a directory) or the old tests fail.

**Step 3: Implement the proxy**

Rewrite `cmd/pokeui/main.go`:

- `handler(wallBase string) http.Handler`
- `GET /` serves embedded `ui/index.html` (stub for now) with `text/html; charset=utf-8` and `Cache-Control: no-store`
- `GET /v1/dashboard`, `POST /v1/specs`, `POST /v1/runs/{id}/cancel`, `GET /frame` forward to `wallBase` + path + raw query, copying method, body, and `Content-Type`
- 5s client timeout
- on transport error: 502, `{"error":"wall unreachable"}`
- force `Cache-Control: no-store` on dashboard and frame responses
- copy upstream status, `Content-Type`, and body otherwise
- `main`: `-http` stays; replace `-dir` with required `-wall` (fatal if empty)

Stub `cmd/pokeui/ui/index.html`:

```html
<!doctype html>
<html><head><meta charset="utf-8"><title>pokefarm</title></head>
<body>pokefarm</body></html>
```

```go
//go:embed ui/index.html
var indexHTML []byte
```

Do not forward `/v1/lease` or any other path.

**Step 4: Run tests to verify GREEN**

```bash
go test ./cmd/pokeui ./cmd/pokewall -count=1
```

Expected: PASS.

**Step 5: Commit** (skip if agent-runner)

```bash
git add cmd/pokeui
git commit -m "$(cat <<'EOF'
Turn pokeui into an allowlisted reverse proxy for the wall.

EOF
)"
```

---

### Task 3: Embedded operator console

**Files:**
- Replace: `cmd/pokeui/ui/index.html`
- Test: `cmd/pokeui/relay_test.go` (`TestPokeuiServesIndex` already requires `pokefarm`; extend it to require `Queue a run`, `workers`, `history` as landmarks)

**Step 1: Extend the index test**

In `TestPokeuiServesIndex`, also require these substrings: `Queue a run`, `id="live"`, `id="workers"`, `id="history"`, `/v1/dashboard`.

Run:

```bash
go test ./cmd/pokeui -run '^TestPokeuiServesIndex$' -count=1 -v
```

Expected: FAIL on the new landmarks.

**Step 2: Implement the page**

Single file, no build step. Visual tokens (do not substitute a generic dark dashboard):

- Page ground `#14180f`, panel `#1e2619`, bezel `#8b8355`, LCD dark `#0f380f`, LCD `#9bbc0f`, cartridge red `#c62828`, cream `#d4c89a`
- Display: [Syne](https://fonts.google.com/specimen/Syne); data: [IBM Plex Mono](https://fonts.google.com/specimen/IBM+Plex+Mono)
- 8px rhythm, pixelated 160×144 screens, one accent (cartridge red) for Cancel / error / running

Structure matching the design:

1. Top bar: `pokefarm` + counts (running / queued / idle workers)
2. Button opens a **Queue a run** panel: fields `run_id`, `planner`, `starter`, `dest`, `seed`, `fps`, `max_rounds`, `max_frames`; POST `/v1/specs`; show wall `error` or “run already active” on 409
3. `#live` card grid: non-`done` runs; screen `<img src="/frame?run=...&t=now">` only when `status === "running"` (`image-rendering: pixelated`); Cancel POSTs `/v1/runs/{id}/cancel`
4. `#workers` chips from `workers[]`
5. `#history` finished runs (`status === "done"`)
6. Click card → detail pane: trace / stop_so_far, or reason + detail + last trace. No save-state.
7. Poll `/v1/dashboard` every 2s; keep selected `run_id` and scroll; refetch immediately after successful queue/cancel
8. Wall down: banner “wall unreachable”, disable the form, keep last snapshot
9. Missing frame: empty bezel, hide broken `<img>` (`onerror` hide)

Copy is sentence case: `Queue a run`, `Cancel run`, `No runs yet`, `No workers`, `Nothing finished yet`.

**Step 3: Run tests**

```bash
go test ./cmd/pokeui ./cmd/pokewall -count=1
```

Expected: PASS.

**Step 4: Commit** (skip if agent-runner)

```bash
git add cmd/pokeui/ui/index.html cmd/pokeui/relay_test.go
git commit -m "$(cat <<'EOF'
Add the embedded pokefarm operator console.

EOF
)"
```

---

### Task 4: Put pokeui in the stack with host-mode publish

**Files:**
- Modify: `deploy/farm.yml`
- Modify: `Makefile` (`farm-up` / `farm-down`, drop frames-dir wiring)
- Modify: `cmd/pokewall/main.go` comments only if they claim pokeui is a file relay
- Modify: `cmd/pokeui/main.go` package comment
- Modify: `deploy/Dockerfile` comment (still three entrypoints)

**Step 1: Stack file**

Add a `ui` service; drop the wall’s `-publish` and frames volume:

```yaml
  wall:
    image: ${FARM_IMAGE:-pokepilot-farm:local}
    command: ["pokewall", "-http", ":8080", "-dumps", "/var/lib/pokewall/dumps", "-state", "/var/lib/pokewall/state.json"]
    deploy:
      replicas: 1
    volumes:
      - ${FARM_STATE_DIR}:/var/lib/pokewall

  ui:
    image: ${FARM_IMAGE:-pokepilot-farm:local}
    command: ["pokeui", "-http", ":8080", "-wall", "http://wall:8080"]
    ports:
      - target: 8080
        published: ${FARM_WALL_PORT:-18080}
        protocol: tcp
        mode: host
    deploy:
      replicas: 1
```

Keep `runner` as it is. Rewrite the file header comment: pokeui is in the stack; host-mode publish skips ingress; wall publishes no host ports.

**Step 2: Makefile**

- Stop `docker rm` / `docker run pokefarm_ui` and the port-retry loop
- `farm-up`: still `mkdir` state dir; frames dir is no longer required
- Force-roll `pokefarm_ui` with the wall and runner
- `farm-down`: `docker stack rm` is enough; keep `docker rm -f pokefarm_ui` once so an old leftover container dies
- Print `pokefarm UI: http://localhost:$(FARM_WALL_PORT)/`
- `FARM_FRAMES_DIR` can go if nothing references it

**Step 3: Verify without deploying Swarm**

```bash
go test ./cmd/pokeui ./cmd/pokewall ./farm ./cmd/pokepilot -count=1
```

Expected: PASS. Do not require `make farm-up` in CI (needs ROM + Swarm).

**Step 4: Commit** (skip if agent-runner)

```bash
git add deploy/farm.yml Makefile cmd/pokeui/main.go cmd/pokewall/main.go deploy/Dockerfile
git commit -m "$(cat <<'EOF'
Serve the farm UI from the stack on a host-mode port.

EOF
)"
```

---

### Task 5: Manual check (operator, not CI)

On a machine with Swarm and a ROM:

```bash
make farm-up
# browser: http://localhost:18080/
# queue a run, see a card, confirm a worker chip, cancel, see history
make farm-down
```

If overlay DNS fails, the page shows “wall unreachable” — that is the designed 502 path, not a silent empty grid.
