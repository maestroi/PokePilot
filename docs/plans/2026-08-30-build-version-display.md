# Build Version Display Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show each worker's build git SHA (plus wall and console SHAs) on the pokefarm wall and operator console, so a deploy's rollout is verifiable at a glance.

**Architecture:** The Dockerfile stamps `main.version` via ldflags from a CI build-arg. Runners carry it on existing presence pings and heartbeats (`farm.Client` stamps both); the wall stores it per worker and serves it in the dashboard JSON + debug HTML; pokeui serves its own SHA at `/v1/version` and the console renders header SHAs, per-chip SHAs, and a version distribution line.

**Tech Stack:** Go (stdlib only), Go text/template, vanilla JS (embedded static console), Docker build-args, GitHub Actions.

Design: `docs/plans/2026-08-30-build-version-display-design.md`

---

### Task 1: Stamp the build SHA into all three binaries

**Files:**
- Modify: `deploy/Dockerfile` (build stage, lines ~34-36)
- Modify: `.github/workflows/publish-farm.yml` (build-push step)
- Modify: `cmd/pokepilot/main.go`, `cmd/pokewall/main.go`, `cmd/pokeui/main.go` (each gets `var version`)

**Step 1: Add the version variable to each binary**

In each of the three `main` packages, add near the top (after imports):

```go
// version is this build's identity (git SHA), stamped by the Dockerfile via
// -ldflags "-X main.version=..."; "dev" for local builds.
var version = "dev"
```

**Step 2: Stamp in the Dockerfile**

In `deploy/Dockerfile`, replace the single `RUN CGO_ENABLED=0 go build ...` line with:

```dockerfile
ARG GIT_SHA=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokepilot ./cmd/pokepilot && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokewall ./cmd/pokewall && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokeui ./cmd/pokeui
```

**Step 3: Pass the SHA from CI**

In `.github/workflows/publish-farm.yml`, add to the `docker/build-push-action@v6` step (next to `tags:`):

```yaml
          build-args: |
            GIT_SHA=${{ github.sha }}
```

**Step 4: Verify builds still work, stamped and unstamped**

Run:
```bash
go build ./... && \
CGO_ENABLED=0 go build -ldflags "-X main.version=deadbeef" -o /tmp/pokeui-stamp ./cmd/pokeui && \
strings /tmp/pokeui-stamp | grep -c deadbeef
```
Expected: builds succeed; `strings` count ≥ 1. (The live check of the stamped value happens in Task 6.)

**Step 5: Commit**

```bash
git add deploy/Dockerfile .github/workflows/publish-farm.yml cmd/pokepilot/main.go cmd/pokewall/main.go cmd/pokeui/main.go
git commit -m "farm: stamp the build SHA into all three binaries"
```

---

### Task 2: farm wire contract — version on ping and heartbeat

**Files:**
- Modify: `farm/spec.go` (`Heartbeat`, `WorkerPing`)
- Modify: `farm/client.go` (`Client`, `Heartbeat`, `Ping`)
- Modify: `cmd/pokepilot/main.go` (farm-mode block, ~line 96)
- Test: `farm/client_test.go`

**Step 1: Write the failing tests**

Append to `farm/client_test.go`:

```go
func TestClientPingSendsVersion(t *testing.T) {
	var got farm.WorkerPing
	srv := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		json.NewDecoder(req.Body).Decode(&got) //nolint:errcheck
		res.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.Version = "abc123"
	if err := c.Ping(context.Background(), []string{"10.0.1.5:8099"}); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if got.Version != "abc123" {
		t.Fatalf("ping version = %q, want abc123", got.Version)
	}
}

func TestClientHeartbeatSendsVersion(t *testing.T) {
	var got farm.Heartbeat
	srv := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		json.NewDecoder(req.Body).Decode(&got) //nolint:errcheck
		json.NewEncoder(res).Encode(HeartbeatReply{}) //nolint:errcheck
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.Version = "abc123"
	if _, err := c.Heartbeat(context.Background(), Heartbeat{RunID: "r1"}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if got.Version != "abc123" {
		t.Fatalf("heartbeat version = %q, want abc123", got.Version)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./farm -run 'Version' -v`
Expected: FAIL (no field `Version` on `WorkerPing`/`Heartbeat`/`Client`).

**Step 3: Implement the contract**

`farm/spec.go` — add to `Heartbeat` (after `WorkerAddrs`):

```go
	// Version is this runner's build identity (git SHA), so the wall can
	// show which build each worker runs. Empty from older runners.
	Version string `json:"version,omitempty"`
```

and to `WorkerPing`:

```go
type WorkerPing struct {
	Addrs   []string `json:"addrs"`
	Version string   `json:"version,omitempty"`
}
```

`farm/client.go` — add the field:

```go
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// Version is this build's identity (git SHA), stamped onto pings and
	// heartbeats so the wall can show which build each worker runs.
	Version string
}
```

In `Heartbeat`, before `json.Marshal(hb)`: `hb.Version = c.Version`
In `Ping`: `body, err := json.Marshal(WorkerPing{Addrs: addrs, Version: c.Version})`

`cmd/pokepilot/main.go` — farm-mode block becomes:

```go
		client := farm.NewClient(orchURL)
		client.Version = version
		runFarm(m, client, bootState, watchPort(served))
```

**Step 4: Run tests to verify they pass**

Run: `go test ./farm ./cmd/pokepilot`
Expected: PASS.

**Step 5: Commit**

```bash
git add farm/spec.go farm/client.go farm/client_test.go cmd/pokepilot/main.go
git commit -m "farm: carry the runner's build SHA on pings and heartbeats"
```

---

### Task 3: Wall records worker versions and exposes its own

**Files:**
- Modify: `cmd/pokewall/wall.go` (`workerInfo`, `workerRow`, `dashboardView`, `upsertWorkerLocked`, `snapshot`, `renderGrid`, `gridTmpl`, both `upsertWorkerLocked` call sites, `Wall` struct)
- Modify: `cmd/pokewall/main.go` (set `wall.Version`)
- Test: `cmd/pokewall/workers_test.go`

**Step 1: Write the failing test**

Append to `cmd/pokewall/workers_test.go`:

```go
func TestWallWorkerVersion(t *testing.T) {
	wall := NewWall(t.TempDir())
	wall.Version = "def456"
	h := wall.Handler()

	body, _ := json.Marshal(farm.WorkerPing{Addrs: []string{"10.0.1.30:8099"}, Version: "abc123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/workers", bytes.NewReader(body))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/workers = %d: %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	var dash struct {
		WallVersion string `json:"wall_version"`
		Workers     []struct {
			Addr    string `json:"addr"`
			Version string `json:"version"`
		} `json:"workers"`
	}
	if err := json.NewDecoder(res.Body).Decode(&dash); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if dash.WallVersion != "def456" {
		t.Fatalf("wall_version = %q, want def456", dash.WallVersion)
	}
	if len(dash.Workers) != 1 || dash.Workers[0].Version != "abc123" {
		t.Fatalf("workers = %+v, want one worker with version abc123", dash.Workers)
	}

	page := gridHTML(t, h)
	for _, want := range []string{"abc123", "def456"} {
		if !strings.Contains(page, want) {
			t.Fatalf("grid missing %q:\n%s", want, page)
		}
	}
}

func TestWallWorkerVersionEmptyFromOldRunner(t *testing.T) {
	wall := NewWall(t.TempDir())
	h := wall.Handler()

	postWorkerPing(t, h, []string{"10.0.1.31:8099"}) // no Version field

	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	body, _ := io.ReadAll(res.Body)
	if strings.Contains(string(body), `"version"`) {
		t.Fatalf("old-runner worker should omit the version key:\n%s", body)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/pokewall -run 'WorkerVersion' -v`
Expected: FAIL (no `Version` field / no `wall_version`).

(Add `"io"` to the test file's imports if not present.)

**Step 3: Implement**

`cmd/pokewall/wall.go`:

- `Wall` struct: add field `Version string // this wall's build identity, shown in the dashboard and grid`.
- `workerInfo`: add `Version string // build identity the runner reports; "" from older runners`.
- `upsertWorkerLocked` signature becomes `(addrs []string, runID, version string, now time.Time)` and stores `Version: version`.
- Call sites: heartbeat handler → `w.upsertWorkerLocked(hb.WorkerAddrs, id, hb.Version, t.lastUpdate)`; `handleWorkers` → `w.upsertWorkerLocked(ping.Addrs, "", ping.Version, now)`.
- `workerRow`: add `Version string \`json:"version,omitempty"\``.
- `snapshot()`: fill `Version: wk.Version` in the worker row; return `dashboardView{Now: now.Unix(), WallVersion: w.Version, Runs: rows, Workers: workers}`.
- `dashboardView`: add `WallVersion string \`json:"wall_version,omitempty"\``.
- `renderGrid`: add `Version string` to the anonymous view struct, filled with `w.Version`.
- `gridTmpl`: header becomes `<h1>pokefarm wall <small>{{.Version}}</small></h1>`; workers table:

```
<tr><th>worker</th><th>version</th><th>status</th><th>seen</th></tr>
{{range .Workers}}<tr><td>{{.Addr}}</td><td>{{if .Version}}{{.Version}}{{else}}&mdash;{{end}}</td><td>{{if .RunID}}running {{.RunID}}{{else}}idle{{end}}</td><td>{{.SeenAgo}} ago</td></tr>
{{else}}<tr><td colspan="4">no workers</td></tr>
```

`cmd/pokewall/main.go`: after `wall := NewWall(*dumpsDir)` add `wall.Version = version`.

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/pokewall`
Expected: PASS (including the pre-existing presence/heartbeat tests).

**Step 5: Commit**

```bash
git add cmd/pokewall/wall.go cmd/pokewall/main.go cmd/pokewall/workers_test.go
git commit -m "wall: show each worker's build SHA and the wall's own"
```

---

### Task 4: pokeui serves its own version

**Files:**
- Modify: `cmd/pokeui/main.go` (route table in `handler`)
- Test: `cmd/pokeui/console_test.go`

**Step 1: Write the failing test**

Append to `cmd/pokeui/console_test.go`:

```go
func TestVersionEndpoint(t *testing.T) {
	h := handler("http://wall.invalid")
	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/version = %d, want 200: %s", res.Code, res.Body.String())
	}
	var got map[string]string
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["version"] == "" {
		t.Fatal("version missing from /v1/version")
	}
}
```

(Adjust imports if `json`/`httptest` are not already imported in that file.)

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/pokeui -run TestVersionEndpoint -v`
Expected: FAIL (404 / no route).

**Step 3: Implement**

In `handler`, next to the other routes:

```go
	mux.HandleFunc("GET /v1/version", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		res.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(res).Encode(map[string]string{"version": version}) //nolint:errcheck
	})
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/pokeui`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/pokeui/main.go cmd/pokeui/console_test.go
git commit -m "pokeui: serve the console's build SHA at /v1/version"
```

---

### Task 5: Console UI — header SHAs, chip SHAs, distribution line

**Files:**
- Modify: `cmd/pokeui/ui/index.html` (header counts, CSS)
- Modify: `cmd/pokeui/ui/ui.js` (startup fetch, `renderCounts`, `renderWorkers`)

No JS test harness exists in this repo; verification is `node --check` plus the live check in Task 6.

**Step 1: Header + CSS in index.html**

In the header `.counts` div, after the idle-workers span, add:

```html
      <span class="ver" id="versions"></span>
```

In the `<style>` block, near the other `.worker` rules (~line 312):

```css
  #versions { color: var(--bezel); font-size: 11px; font-family: ui-monospace, monospace; }
  .worker .ver { color: var(--bezel); font-family: ui-monospace, monospace; font-size: 11px; }
  .ver-summary { color: var(--cream); font-family: ui-monospace, monospace; font-size: 12px; margin-bottom: 6px; }
```

**Step 2: JS in ui.js**

At the top of the IIFE, next to `let snap = ...`:

```js
  let consoleVersion = "";
```

Startup (next to the final `refresh(); setInterval(...)`):

```js
  fetch("/v1/version", { cache: "no-store" })
    .then((r) => (r.ok ? r.json() : null))
    .then((v) => { if (v && v.version) { consoleVersion = v.version; renderVersions(); } })
    .catch(() => {}); // version is cosmetic; never block the console on it
```

New function (near `renderCounts`):

```js
  function renderVersions() {
    const wall = snap.wall_version || "";
    $("versions").textContent =
      ["console", consoleVersion, "wall", wall].filter(Boolean).join(" · ");
  }
```

Call `renderVersions()` inside `render()` (after `renderCounts()`), so a late wall_version from the first dashboard refresh still lands.

In `renderWorkers`, before building chips, compute the distribution and prepend it:

```js
    const byVer = {};
    for (const w of ws) {
      const v = w.version || "unknown";
      byVer[v] = (byVer[v] || 0) + 1;
    }
    const summary = Object.entries(byVer)
      .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
      .map(([v, n]) => `${n} × ${v}`)
      .join(", ");
```

and render it as the first child:

```js
    el.innerHTML = `<p class="ver-summary">${esc(summary)}</p>` + ws.map((w) => {
      ...
      return `<div class="worker">
        ${chip(busy ? "busy" : "idle", busy ? "busy" : "idle")}
        <span class="addr">${esc(w.addr)}</span>
        ${w.version ? `<span class="ver">${esc(w.version)}</span>` : ""}
        <span class="job">${job}</span>
        <span class="ago">${esc(w.seen_ago)} ago</span>
      </div>`;
    }).join("");
```

**Step 3: Syntax-check**

Run: `node --check cmd/pokeui/ui/ui.js`
Expected: no output (OK).

**Step 4: Commit**

```bash
git add cmd/pokeui/ui/index.html cmd/pokeui/ui/ui.js
git commit -m "console: show build SHAs for console, wall and every worker"
```

---

### Task 6: Deploy and verify the fleet (acceptance test)

**Step 1: Push and wait for the image**

```bash
git push origin main
gh run list --branch main --limit 1   # note the run id for the new SHA
gh run watch <RUN_ID> --exit-status
```
Expected: success.

**Step 2: Roll the fleet**

```bash
./deploy/swarm.sh pull
```
Expected: `updated 3 service(s) to sha256:…` (new digest).

**Step 3: Verify live**

```bash
SHA=$(git rev-parse --short HEAD)
curl -s https://pokemon.labstack.cc/v1/version | jq -r .version
curl -s https://pokemon.labstack.cc/v1/dashboard | jq '{wall_version, workers: [.workers[] | {addr, version}]}'
```
Expected: console version = `$SHA`; `wall_version` = `$SHA`; **all ten** worker versions = `$SHA`. Open https://pokemon.labstack.cc and confirm the header shows both SHAs and the Workers section reads `10 × $SHA`.

If any worker shows a different SHA or `unknown`, that runner has not rolled — check `docker service ps pokefarm_runner` on the manager for stragglers.
