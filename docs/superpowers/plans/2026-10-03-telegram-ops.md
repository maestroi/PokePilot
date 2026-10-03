# Telegram Ops Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Telegram bot the single phone-friendly ops surface for the farm: tap-driven cards, alerts with mute/resolve, a pinned live board, a daily digest, a "flag stuck" action, and every check `deploy/farm-watch.sh` used to run, all as Swarm services.

**Architecture:** Three services from the farm image in stack `pokefarm-ops`: `telegram` (unprivileged bot: Telegram I/O, alert state machine, cards), `watch` (`cmd/pokewatch`, one replica on a manager with the Docker socket: Swarm/GitHub checks, deploy freeze), and `node` (`pokewatch -node`, global, host `/` read-only: disk and fixer ledger). `node` → `watch` → `telegram` over HTTP with a shared bearer token. Flag-stuck is a wall-only change that rewrites a flagged run's cooperative-cancel finish into the existing `stuck` outcome.

**Tech Stack:** Go 1.27 stdlib only (net/http over a unix socket for Docker, GitHub REST, Telegram Bot API), bash for `rollout-latest.sh`, Docker Swarm compose v3.

**Spec:** `docs/superpowers/specs/2026-10-03-telegram-ops-design.md`

## Global Constraints

- No new Go module dependencies. Docker and GitHub are called with `net/http`.
- `telegram` gets no Docker socket, no SSH, no host mounts (spec: "keeps the documented no-socket/no-SSH safety model").
- Nothing game-specific in bot, watcher or wall changes (`docs/ARCHITECTURE.md`); map names come only from the run's generic `game_state.map_name` field when present.
- Flag-stuck reuses the existing `stuck` outcome; triage, issues and fixer code are not changed.
- Alert defaults: reminder every 12h, mute 12h, disk threshold `POKEPILOT_WATCH_MIN_FREE_GB` default 20, paid cap `POKEPILOT_PAID_DAILY_CAP` default 20, freeze after 3 rollbacks in 24h for 24h, flag fallback 10 min, watcher-silent after 5 min, digest hour `POKEPILOT_WATCH_DIGEST_HOUR` default 9.
- Internal endpoints require `Authorization: Bearer <token>` read from `POKEPILOT_OPS_TOKEN_FILE` (Swarm secret `pokefarm_ops_token`).
- Callback data must stay ≤ 64 bytes (Telegram limit); run IDs are never put in callback data directly.
- Every new Go file carries tests in the same package; run `make fmt-check`, `go vet ./...` and `go test -short -count=1 ./...` before the final commit (see memory: unset partial `POKEPILOT_S3_*` env before `make test-short`).

## Review Focus

1. **Telegram rejects an edit** ("message is not modified", "message to edit not found", message older than 48h): the bot must not crash, must not loop; for alerts it falls back to sending a new message; for boards it drops the board. → test in Task 7.
2. **HTML special characters in run IDs, goals, error text** (`<`, `>`, `&`): every interpolated string is escaped or Telegram rejects the whole message. → test in Task 7.
3. **Bot restart while checks are failing**: one summary message, no per-check page storm, and the adopted alerts still resolve later. → test in Task 6.
4. **A node stops reporting** (node agent crash or node down): watch raises `node-report:<node>` instead of silently dropping its disk check. → test in Task 4.
5. **Flag on a run that finishes for another reason first** (goal done, error): the finish must not be rewritten unless it is the cooperative-cancel stop (`budget` with empty detail, or `cancelled`). → test in Task 1.

---

## File Structure

| File | Responsibility |
|---|---|
| `cmd/pokewall/operator_flag.go` (new) | flag-stuck endpoint, finish rewrite, reaper fallback |
| `cmd/pokewall/wall.go` (modify) | route, Tile fields, hooks in `handleFinish`/`settleRun`/`reapStale` |
| `cmd/pokewall/control_plane_finish.go` (modify) | apply the same rewrite before `persistFinish` |
| `operatorapi/ops.go` (new) | shared wire types `CheckResult`, `NodeReport`, `OpsSnapshot`…; `Client.FlagStuck` |
| `cmd/pokewatch/main.go` (new) | flags/config, `-node` vs watch mode, loops |
| `cmd/pokewatch/node.go` (new) | disk statfs + fixer ledger summary |
| `cmd/pokewatch/docker.go` (new) | minimal Docker Engine API client over unix socket |
| `cmd/pokewatch/github.go` (new) | GitHub search client |
| `cmd/pokewatch/watch.go` (new) | check evaluation, freeze, snapshot push |
| `deploy/rollout-latest.sh` (modify) | honor `pokepilot.deploy-frozen-until` |
| `cmd/poketelegram/alerts.go` (new) | alert state machine (`alertBook`) |
| `cmd/poketelegram/telegram.go` (new) | Telegram client moved out of main.go + `send`/`edit`/`setCommands` |
| `cmd/poketelegram/ui.go` (new) | callback router, handles, cards, paging, reply/number resolution |
| `cmd/poketelegram/ops.go` (new) | `/v1/ops` ingest, local checks, alert rendering, board, digest, flag follow-up |
| `cmd/poketelegram/main.go` (modify) | wire new pieces, drop old stall/wall notifications |
| `deploy/ops.yml` (new, replaces `deploy/telegram.yml`) | the three services |
| `deploy/Dockerfile` (modify) | build `pokewatch` |
| `deploy/ops_config_test.go` (new) | `docker stack config` sanity like other stack tests |
| `docs/TELEGRAM_OPERATOR.md`, `deploy/README.md`, `Makefile` (modify) | docs; delete farm-watch + desktop timer install |
| `deploy/farm-watch.*`, `deploy/farm_watch_test.go`, `deploy/qwagent-triage.{service.in,timer,zsh}` (delete) | decommissioned |

---

### Task 1: Wall flag-stuck

**Files:**
- Create: `cmd/pokewall/operator_flag.go`, `cmd/pokewall/operator_flag_test.go`
- Modify: `cmd/pokewall/wall.go` (Tile struct near line 46, route near line 659, `handleFinish` near line 1162, `settleRun` near line 1810, `reapStale` near line 2029), `cmd/pokewall/control_plane_finish.go` (near line 172)
- Modify: `operatorapi/client.go` (add `FlagStuck`)

**Interfaces:**
- Produces: `POST /v1/runs/{id}/flag-stuck` body `{"note":"..."}` → 200 `{"flagged":true,"attempt":N}`, 404 unknown, 409 finished.
- Produces: `func (c *Client) FlagStuck(ctx context.Context, id, note string) error` in `operatorapi`.
- Produces: Tile JSON fields `operator_flag`, `operator_flag_attempt`, `operator_flagged_at` (visible on `GET /v1/runs/{id}` and the dashboard).

- [ ] **Step 1: Write the failing tests**

`cmd/pokewall/operator_flag_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func flagTestWall(t *testing.T) (*Wall, string) {
	t.Helper()
	w := newTestWall(t)
	id := startTestRun(t, w) // leased + one heartbeat; attempt 1 in flight
	return w, id
}

func postFlag(t *testing.T, w *Wall, id, note string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"note": note})
	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/runs/"+id+"/flag-stuck", bytes.NewReader(body)))
	return rec
}

func TestFlagStuckRequestsCancelAndRecordsAttempt(t *testing.T) {
	w, id := flagTestWall(t)
	if rec := postFlag(t, w, id, "looping in menu"); rec.Code != http.StatusOK {
		t.Fatalf("flag: %d %s", rec.Code, rec.Body)
	}
	w.mu.Lock()
	tile, cancel := w.tiles[id], w.cancel[id]
	w.mu.Unlock()
	if !cancel {
		t.Fatal("flag must ask the runner to stop through the cooperative cancel flag")
	}
	if tile.OperatorFlag != "looping in menu" || tile.OperatorFlagAttempt != 1 || tile.OperatorFlaggedAt == 0 {
		t.Fatalf("tile flag fields: %+v", tile)
	}
	if rec := postFlag(t, w, "nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown run: %d", rec.Code)
	}
}

func TestFlaggedCancelFinishBecomesStuck(t *testing.T) {
	w, id := flagTestWall(t)
	postFlag(t, w, id, "")
	report := farm.FinishReport{RunID: id, Attempt: 1, Reason: "budget"}
	w.operatorFlagFinish(&report)
	if report.Reason != "stuck" || report.Detail != "operator flagged: no note" {
		t.Fatalf("rewrite: %q %q", report.Reason, report.Detail)
	}
	finishTestRun(t, w, report)
	w.mu.Lock()
	tile := w.tiles[id]
	w.mu.Unlock()
	if tile.Reason == "cancelled" {
		t.Fatal("a flagged run must not settle as a user cancel")
	}
}

func TestFlagDoesNotRewriteOtherOutcomes(t *testing.T) {
	w, id := flagTestWall(t)
	postFlag(t, w, id, "x")
	for _, r := range []farm.FinishReport{
		{RunID: id, Attempt: 1, Reason: "done"},
		{RunID: id, Attempt: 1, Reason: "error", Detail: "boom"},
		{RunID: id, Attempt: 1, Reason: "budget", Detail: "frame budget exhausted"},
		{RunID: id, Attempt: 2, Reason: "budget"},
	} {
		before := r
		w.operatorFlagFinish(&r)
		if r.Reason != before.Reason || r.Detail != before.Detail {
			t.Fatalf("rewrote %+v into %+v", before, r)
		}
	}
}

func TestReaperSettlesFlaggedRunThatNeverStops(t *testing.T) {
	w, id := flagTestWall(t)
	postFlag(t, w, id, "wedged")
	w.mu.Lock()
	w.tiles[id].OperatorFlaggedAt = time.Now().Add(-11 * time.Minute).Unix()
	w.tiles[id].lastUpdate = time.Now() // still heartbeating
	w.mu.Unlock()
	w.reapStale(time.Now())
	w.mu.Lock()
	tile := w.tiles[id]
	w.mu.Unlock()
	if tile.Attempts != 1 {
		t.Fatalf("attempt not settled: %+v", tile)
	}
	last := tile.Activity[len(tile.Activity)-1]
	if !strings.Contains(last.Detail, "runner did not stop within 10m") {
		t.Fatalf("activity detail: %+v", last)
	}
}
```

Before writing, look at existing wall tests (`grep -n "func newTestWall\|func startTestRun\|func finishTestRun" cmd/pokewall/*_test.go`). Reuse the helpers that exist under those or similar names. If `startTestRun`/`finishTestRun` do not exist, add them to `operator_flag_test.go` using the same enqueue → lease → heartbeat → finish HTTP calls the existing wall tests make (copy one existing test's setup, for example the one that exercises `handleCancel`). `tile.Activity` is the field `appendRunActivityLocked` writes; use its real name from the Tile struct.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/pokewall/ -run 'FlagStuck|Flagged|FlagDoesNot|ReaperSettlesFlagged' -count=1`
Expected: FAIL to compile (`OperatorFlag`, `operatorFlagFinish` undefined).

- [ ] **Step 3: Implement**

Tile fields (add to `type Tile struct` in `wall.go`):

```go
	// OperatorFlag is the note an operator attached when flagging this run
	// as stuck ("no note" when none). OperatorFlagAttempt is the attempt the
	// flag applies to, so a duplicate finish after settlement rewrites the
	// same way and a later attempt is never affected.
	OperatorFlag        string `json:"operator_flag,omitempty"`
	OperatorFlagAttempt int    `json:"operator_flag_attempt,omitempty"`
	OperatorFlaggedAt   int64  `json:"operator_flagged_at,omitempty"`
```

Route next to cancel: `mux.HandleFunc("POST /v1/runs/{id}/flag-stuck", w.handleFlagStuck)`.

`cmd/pokewall/operator_flag.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

// operatorFlagGrace bounds how long a flagged run may keep heartbeating
// without finishing before the reaper settles it as stuck itself.
const operatorFlagGrace = 10 * time.Minute

// handleFlagStuck lets an operator declare an active run stuck. The runner is
// stopped through the cooperative cancel flag; operatorFlagFinish then turns
// that stop into the ordinary stuck outcome, so the failure reaches triage
// and the fixer exactly like a stagnation-watchdog stop.
func (w *Wall) handleFlagStuck(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var body struct {
		Note string `json:"note"`
	}
	req.Body = http.MaxBytesReader(res, req.Body, 4<<10)
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad flag body: " + err.Error()})
		return
	}
	note := strings.TrimSpace(body.Note)
	if note == "" {
		note = "no note"
	}
	if len(note) > 500 {
		note = note[:500]
	}
	w.mu.Lock()
	t, ok := w.tiles[id]
	if !ok {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + id})
		return
	}
	if t.Finished {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run already finished: " + id})
		return
	}
	now := time.Now()
	t.OperatorFlag = note
	t.OperatorFlagAttempt = t.Attempts + 1
	t.OperatorFlaggedAt = now.Unix()
	w.cancel[id] = true
	appendRunActivityLocked(t, runActivityEvent{
		Source: "operator", Kind: "flag-stuck", At: now.Unix(), Frame: t.Frame, Attempt: t.OperatorFlagAttempt,
		Summary: "Operator flagged run as stuck", Detail: note,
	})
	attempt := t.OperatorFlagAttempt
	w.mu.Unlock()
	w.saveState()
	writeJSON(res, http.StatusOK, map[string]any{"flagged": true, "attempt": attempt})
}

// operatorFlagFinish rewrites the cooperative-cancel stop of a flagged
// attempt into the stuck outcome. Only the stop the flag caused is
// rewritten: the LLM path reports a cancel as budget with no detail, the
// policy paths report cancelled. Any other outcome stands.
func (w *Wall) operatorFlagFinish(report *farm.FinishReport) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tiles[report.RunID]
	if !ok || t.OperatorFlag == "" {
		return
	}
	attempt := report.Attempt
	if attempt == 0 {
		attempt = t.Attempts + 1
		if t.Finished {
			attempt = t.Attempts
		}
	}
	if attempt != t.OperatorFlagAttempt {
		return
	}
	cancelStop := (report.Reason == "budget" && strings.TrimSpace(report.Detail) == "") || report.Reason == "cancelled"
	if !cancelStop {
		return
	}
	report.Reason = "stuck"
	report.Detail = "operator flagged: " + t.OperatorFlag
}

// reapFlaggedLocked settles a flagged attempt whose runner keeps
// heartbeating but never finishes (a wedged loop cannot honor the cancel).
// There is no finish dump in that case, so no issue is filed; the activity
// entry says so. Caller holds w.mu.
func (w *Wall) reapFlaggedLocked(t *Tile, now time.Time) bool {
	if t.Finished || t.Status == statusQueued || t.OperatorFlag == "" || t.OperatorFlagAttempt != t.Attempts+1 {
		return false
	}
	if now.Sub(time.Unix(t.OperatorFlaggedAt, 0)) < operatorFlagGrace {
		return false
	}
	w.settleRun(t, "stuck", fmt.Sprintf("operator flagged: %s (runner did not stop within %s; no finish dump)", t.OperatorFlag, operatorFlagGrace))
	return true
}
```

In `handleFinish`, right after the `FailureClass` validation and before `w.mu.Lock()`:

```go
	w.operatorFlagFinish(&report)
```

In `control_plane_finish.go`, inside `if json.Unmarshal(data, &parsed) == nil {`, before `finish = &parsed`:

```go
				w.operatorFlagFinish(&parsed)
```

(The inner `handleFinish` re-decodes the original body and applies the same deterministic rewrite, so both paths agree.)

In `settleRun`, replace

```go
	_, cancelled := w.cancel[t.RunID]
```

with

```go
	_, cancelled := w.cancel[t.RunID]
	// A flag stops the runner through the cancel flag, but it is a stuck
	// outcome, not a user cancel: recovery must still apply.
	if t.OperatorFlag != "" && t.OperatorFlagAttempt == completed {
		cancelled = false
	}
```

In `reapStale`, inside the loop after the `Finished/queued` skip and before the age check:

```go
		if w.reapFlaggedLocked(t, now) {
			reaped = append(reaped, id)
			continue
		}
```

`operatorapi/client.go`:

```go
// FlagStuck asks the wall to stop an active run as stuck so the failure
// reaches triage like a stagnation-watchdog stop.
func (c *Client) FlagStuck(ctx context.Context, id, note string) error {
	return c.wallJSON(ctx, http.MethodPost, runPath(id)+"/flag-stuck", map[string]string{"note": note}, &map[string]any{})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/pokewall/ ./operatorapi/ -count=1 -short`
Expected: PASS (new tests plus every existing wall test).

- [ ] **Step 5: Commit**

```bash
git add cmd/pokewall/operator_flag.go cmd/pokewall/operator_flag_test.go cmd/pokewall/wall.go cmd/pokewall/control_plane_finish.go operatorapi/client.go
git commit -m "feat(wall): operator flag-stuck turns a run's cancel stop into the stuck outcome"
```

---

### Task 2: Shared ops wire types

**Files:**
- Create: `operatorapi/ops.go`, `operatorapi/ops_test.go`

**Interfaces:**
- Produces (used by Tasks 3–9):

```go
type CheckResult struct {
	Name    string `json:"name"`             // stable id, e.g. "disk:vm-swarm-worker-03:/"
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Grace   int    `json:"grace"`            // consecutive bad observations before paging
	RunID   string `json:"run_id,omitempty"` // set when the check concerns one run
	Link    string `json:"link,omitempty"`
}
type DiskFree struct {
	Mount  string `json:"mount"`
	FreeGB int    `json:"free_gb"`
}
type FixerSummary struct {
	PaidStarts24h int      `json:"paid_starts_24h"`
	FreeStarts24h int      `json:"free_starts_24h"`
	PaidCap       int      `json:"paid_cap"`
	Attempts24h   int      `json:"attempts_24h"`
	PRs24h        int      `json:"prs_24h"`
	BlockedKeys   []string `json:"blocked_keys,omitempty"`
}
type NodeReport struct {
	Node  string        `json:"node"`
	At    int64         `json:"at"`
	Disks []DiskFree    `json:"disks"`
	Fixer *FixerSummary `json:"fixer,omitempty"`
}
type SwarmNode struct {
	Hostname      string `json:"hostname"`
	Manager       bool   `json:"manager"`
	Status        string `json:"status"`         // ready, down, ...
	ManagerStatus string `json:"manager_status"` // reachable, unreachable, leader ("" for workers)
}
type ServiceState struct {
	Name        string `json:"name"`
	Running     int    `json:"running"`
	Desired     int    `json:"desired"`
	UpdateState string `json:"update_state,omitempty"`
}
type PullRequest struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"created_at"`
}
type OpsSnapshot struct {
	At          int64          `json:"at"`
	Checks      []CheckResult  `json:"checks"`
	Nodes       []SwarmNode    `json:"nodes"`
	Services    []ServiceState `json:"services"`
	Disks       []NodeReport   `json:"disks"`
	Fixer       *FixerSummary  `json:"fixer,omitempty"`
	FrozenUntil int64          `json:"frozen_until,omitempty"`
	TriagePRs   []PullRequest  `json:"triage_prs,omitempty"`
	Merged24h   int            `json:"merged_24h"`   // -1 = unknown
	FarmOpened  int            `json:"farm_opened"`  // -1 = unknown
	FarmClosed  int            `json:"farm_closed"`  // -1 = unknown
}

// PostOps sends v to url with the shared bearer token.
func PostOps(ctx context.Context, client *http.Client, url, token string, v any) error
// OpsAuthorized reports whether req carries the shared bearer token.
func OpsAuthorized(req *http.Request, token string) bool
// ReadSecretFile returns the trimmed contents of path, or "" when unset/unreadable.
func ReadSecretFile(path string) string
```

- [ ] **Step 1: Write the failing test**

```go
package operatorapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPostOpsCarriesBearerAndBody(t *testing.T) {
	var got NodeReport
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !OpsAuthorized(r, "s3cret") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	want := NodeReport{Node: "n1", At: 7, Disks: []DiskFree{{Mount: "/", FreeGB: 12}}}
	if err := PostOps(context.Background(), srv.Client(), srv.URL, "s3cret", want); err != nil {
		t.Fatal(err)
	}
	if got.Node != "n1" || got.Disks[0].FreeGB != 12 {
		t.Fatalf("got %+v", got)
	}
	if err := PostOps(context.Background(), srv.Client(), srv.URL, "wrong", want); err == nil {
		t.Fatal("wrong token must fail")
	}
}

func TestOpsAuthorizedRejectsEmptyToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Authorization", "Bearer ")
	if OpsAuthorized(r, "") {
		t.Fatal("an unset token must never authorize")
	}
}

func TestReadSecretFileTrims(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tok")
	_ = os.WriteFile(p, []byte("abc\n"), 0o600)
	if ReadSecretFile(p) != "abc" || ReadSecretFile("") != "" {
		t.Fatal("ReadSecretFile")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./operatorapi/ -run 'PostOps|OpsAuthorized|ReadSecretFile' -count=1`
Expected: FAIL to compile.

- [ ] **Step 3: Implement** `operatorapi/ops.go` with the types above plus:

```go
func PostOps(ctx context.Context, client *http.Client, url, token string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("POST %s: %s", url, res.Status)
	}
	return nil
}

func OpsAuthorized(req *http.Request, token string) bool {
	if token == "" {
		return false
	}
	got := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func ReadSecretFile(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
```

- [ ] **Step 4: Run tests** — `go test ./operatorapi/ -count=1` → PASS.

- [ ] **Step 5: Commit**

```bash
git add operatorapi/ops.go operatorapi/ops_test.go
git commit -m "feat(operatorapi): shared ops wire types for watcher and bot"
```

---

### Task 3: `pokewatch -node` (disk + fixer ledger)

**Files:**
- Create: `cmd/pokewatch/main.go`, `cmd/pokewatch/node.go`, `cmd/pokewatch/node_test.go`

**Interfaces:**
- Consumes: `operatorapi.NodeReport`, `DiskFree`, `FixerSummary`, `PostOps`, `ReadSecretFile`; `deploy.ParseLadder`, `deploy.ReadLedger`, `deploy.BlockedKeys`, `deploy.BackendBase`.
- Produces: `func nodeReport(host, root string, mounts []string, ledgerPath, ladder string, paidCap int, now time.Time) operatorapi.NodeReport`; `func fixerSummary(rows []deploy.LedgerRow, tiers []deploy.LadderTier, paidCap int, now time.Time) operatorapi.FixerSummary`.
- Env (node mode): `NODE_NAME` (compose sets `{{.Node.Hostname}}`), `POKEWATCH_HOST_ROOT` (default `/host`), `POKEWATCH_MOUNTS` (default `/`), `POKEPILOT_FIXER_LEDGER` (default `/opt/pokefixer/state/ledger.tsv`), `POKEPILOT_TRIAGE_LADDER` (default `opencode:2,cursor:2,cursor/claude-opus-5-5-high:2`), `POKEPILOT_PAID_DAILY_CAP` (default 20), `POKEWATCH_URL` (default `http://watch:8080`), `POKEPILOT_OPS_TOKEN_FILE`, `POKEWATCH_NODE_INTERVAL` (default `5m`).

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/deploy"
)

func TestFixerSummaryCountsPaidFreeAndBlocked(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ts := func(ago time.Duration) string { return strconvI(now.Add(-ago).Unix()) }
	ledger := strings.Join([]string{
		ts(time.Hour) + "\tk1\topencode\tstarted",
		ts(time.Hour) + "\tk1\topencode\tstarted",
		ts(50 * time.Minute) + "\tk1\tcursor\tstarted",
		ts(40 * time.Minute) + "\tk1\tcursor\tstarted",
		ts(30 * time.Minute) + "\tk2\tcursor\tstarted",
		ts(20 * time.Minute) + "\tk2\tcursor\tpr",
		ts(30 * time.Hour) + "\tk3\tcursor\tstarted", // outside 24h
	}, "\n")
	rows := deploy.ReadLedger(strings.NewReader(ledger))
	tiers, _ := deploy.ParseLadder("opencode:2,cursor:2")
	got := fixerSummary(rows, tiers, 20, now)
	if got.FreeStarts24h != 2 || got.PaidStarts24h != 3 || got.Attempts24h != 5 || got.PRs24h != 1 || got.PaidCap != 20 {
		t.Fatalf("counts: %+v", got)
	}
	if len(got.BlockedKeys) != 1 || got.BlockedKeys[0] != "k1" {
		t.Fatalf("k1 spent every tier; blocked = %v", got.BlockedKeys)
	}
}

func TestNodeReportReadsDiskAndOptionalLedger(t *testing.T) {
	root := t.TempDir()
	r := nodeReport("n1", root, []string{"/"}, "/opt/pokefixer/state/ledger.tsv", "opencode:2", 20, time.Now())
	if r.Node != "n1" || len(r.Disks) != 1 || r.Disks[0].Mount != "/" || r.Fixer != nil {
		t.Fatalf("no ledger on this node: %+v", r)
	}
	p := filepath.Join(root, "opt/pokefixer/state/ledger.tsv")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(""), 0o644)
	r = nodeReport("n1", root, []string{"/"}, "/opt/pokefixer/state/ledger.tsv", "opencode:2", 20, time.Now())
	if r.Fixer == nil {
		t.Fatal("ledger present: fixer summary expected")
	}
}

func strconvI(v int64) string { return strconv.FormatInt(v, 10) }
```

(add `"strconv"` to the imports.)

- [ ] **Step 2: Run** `go test ./cmd/pokewatch/ -count=1` → FAIL to compile.

- [ ] **Step 3: Implement** `cmd/pokewatch/node.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/maestroi/pokepilot/deploy"
	"github.com/maestroi/pokepilot/operatorapi"
)

// nodeReport is one node's local facts: free space on each mount (read
// through the read-only host root) and, on the fixer node, the ledger summary.
func nodeReport(host, root string, mounts []string, ledgerPath, ladder string, paidCap int, now time.Time) operatorapi.NodeReport {
	r := operatorapi.NodeReport{Node: host, At: now.Unix()}
	for _, m := range mounts {
		var st syscall.Statfs_t
		if err := syscall.Statfs(filepath.Join(root, m), &st); err != nil {
			continue
		}
		r.Disks = append(r.Disks, operatorapi.DiskFree{Mount: m, FreeGB: int(st.Bavail * uint64(st.Bsize) >> 30)})
	}
	f, err := os.Open(filepath.Join(root, ledgerPath))
	if err != nil {
		return r
	}
	defer f.Close()
	tiers, err := deploy.ParseLadder(ladder)
	if err != nil {
		return r
	}
	s := fixerSummary(deploy.ReadLedger(f), tiers, paidCap, now)
	r.Fixer = &s
	return r
}

// fixerSummary mirrors the retired farm-watch.sh: paid/free starts in the
// rolling 24h and the keys the ladder refuses. The paid cap is passed as
// unlimited to BlockedKeys so a capped day does not mark every key blocked.
func fixerSummary(rows []deploy.LedgerRow, tiers []deploy.LadderTier, paidCap int, now time.Time) operatorapi.FixerSummary {
	s := operatorapi.FixerSummary{PaidCap: paidCap}
	paid := map[string]bool{}
	available := map[string]bool{}
	for _, t := range tiers {
		paid[t.Backend] = t.Paid
		available[deploy.BackendBase(t.Backend)] = true
	}
	for _, r := range rows {
		if now.Sub(r.At) >= 24*time.Hour {
			continue
		}
		switch r.Event {
		case "started":
			s.Attempts24h++
			if paid[r.Backend] {
				s.PaidStarts24h++
			} else {
				s.FreeStarts24h++
			}
		case "pr":
			s.PRs24h++
		}
	}
	s.BlockedKeys = deploy.BlockedKeys(rows, tiers, map[string]int{}, available, 1_000_000, now)
	return s
}
```

Before relying on `available[deploy.BackendBase(...)]`, read `NextBackend` in `deploy/fixer_ladder.go` to confirm whether `available` is keyed by base backend or full tier name, and key the map the same way (farm-watch passed `opencode,cursor,claude`, i.e. bases).

`cmd/pokewatch/main.go` (node half; Task 4 adds watch mode to the same `main`):

```go
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	nodeMode := flag.Bool("node", false, "run the per-node reporter instead of the watcher")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	token := operatorapi.ReadSecretFile(os.Getenv("POKEPILOT_OPS_TOKEN_FILE"))
	if token == "" {
		log.Fatal("pokewatch: POKEPILOT_OPS_TOKEN_FILE is required")
	}
	if *nodeMode {
		runNode(ctx, token)
		return
	}
	runWatch(ctx, token)
}

func runNode(ctx context.Context, token string) {
	host := env("NODE_NAME", "")
	if host == "" {
		host, _ = os.Hostname()
	}
	root := env("POKEWATCH_HOST_ROOT", "/host")
	mounts := strings.Split(env("POKEWATCH_MOUNTS", "/"), ",")
	ledger := env("POKEPILOT_FIXER_LEDGER", "/opt/pokefixer/state/ledger.tsv")
	ladder := env("POKEPILOT_TRIAGE_LADDER", "opencode:2,cursor:2,cursor/claude-opus-5-5-high:2")
	paidCap := envInt("POKEPILOT_PAID_DAILY_CAP", 20)
	target := env("POKEWATCH_URL", "http://watch:8080") + "/v1/node-report"
	client := &http.Client{Timeout: 20 * time.Second}
	tick := time.NewTicker(envDuration("POKEWATCH_NODE_INTERVAL", 5*time.Minute))
	defer tick.Stop()
	for {
		r := nodeReport(host, root, mounts, ledger, ladder, paidCap, time.Now())
		if err := operatorapi.PostOps(ctx, client, target, token, r); err != nil {
			log.Printf("pokewatch node: report: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, err := time.ParseDuration(env(key, "")); err == nil && v > 0 {
		return v
	}
	return fallback
}
```

Add a temporary `func runWatch(ctx context.Context, token string) { <-ctx.Done() }` in `main.go` so the package builds; Task 4 replaces it.

- [ ] **Step 4: Run** `go test ./cmd/pokewatch/ -count=1` → PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/pokewatch/
git commit -m "feat(pokewatch): per-node disk and fixer ledger reporter"
```

---

### Task 4: `pokewatch` watcher (Swarm, GitHub, freeze, push)

**Files:**
- Create: `cmd/pokewatch/docker.go`, `cmd/pokewatch/github.go`, `cmd/pokewatch/watch.go`, `cmd/pokewatch/watch_test.go`
- Modify: `cmd/pokewatch/main.go` (replace the `runWatch` stub)

**Interfaces:**
- Consumes: Task 2 types; Task 3 `NodeReport` posts.
- Produces: HTTP `POST /v1/node-report` (bearer), `GET /healthz`; pushes `operatorapi.OpsSnapshot` to `POKEPILOT_TELEGRAM_URL` (default `http://telegram:8080`) + `/v1/ops` every `POKEWATCH_INTERVAL` (default `1m`).
- Produces: `func (w *watcher) evaluate(now time.Time, nodes []operatorapi.SwarmNode, services []operatorapi.ServiceState, rollbacks []rollbackEvent) operatorapi.OpsSnapshot` (pure; tested directly).
- Check names (consumed by the bot for display only): `node:<host>`, `quorum`, `service:<name>`, `rollback:<name>`, `deploy-frozen`, `disk:<host>:<mount>`, `node-report:<host>`, `paid-cap`, `blocked-keys`, `stuck-prs`.
- Env: `POKEWATCH_STACKS` (default `pokefarm,pokefixer`), `POKEPILOT_GITHUB_REPO` (default `maestroi/PokePilot`), `POKEPILOT_GITHUB_TOKEN_FILE`, `POKEPILOT_WATCH_MIN_FREE_GB` (20), `POKEPILOT_FREEZE_ROLLBACKS` (3), `POKEPILOT_FREEZE_SECONDS` (86400), `POKEWATCH_FREEZE_SERVICE` (default `pokefarm_wall`), `DOCKER_SOCKET` (default `/var/run/docker.sock`).

- [ ] **Step 1: Write the failing tests** (`cmd/pokewatch/watch_test.go`)

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func check(s operatorapi.OpsSnapshot, name string) (operatorapi.CheckResult, bool) {
	for _, c := range s.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return operatorapi.CheckResult{}, false
}

func TestEvaluateFlagsUnreachableManagerAndQuorum(t *testing.T) {
	w := newWatcherForTest()
	now := time.Unix(1_800_000_000, 0)
	s := w.evaluate(now, []operatorapi.SwarmNode{
		{Hostname: "m1", Manager: true, Status: "ready", ManagerStatus: "leader"},
		{Hostname: "m2", Manager: true, Status: "ready", ManagerStatus: "reachable"},
		{Hostname: "m3", Manager: true, Status: "ready", ManagerStatus: "unreachable"},
		{Hostname: "w1", Status: "down"},
	}, nil, nil)
	if c, _ := check(s, "node:m3"); c.OK || !strings.Contains(c.Message, "unreachable") {
		t.Fatalf("m3: %+v", c)
	}
	if c, _ := check(s, "node:w1"); c.OK {
		t.Fatalf("w1 down: %+v", c)
	}
	if c, _ := check(s, "quorum"); c.OK || !strings.Contains(c.Message, "2/3") {
		t.Fatalf("quorum: %+v", c)
	}
}

func TestEvaluateReplicasAndRollback(t *testing.T) {
	w := newWatcherForTest()
	s := w.evaluate(time.Now(), nil, []operatorapi.ServiceState{
		{Name: "pokefarm_wall", Running: 1, Desired: 1},
		{Name: "pokefixer_fixer", Running: 1, Desired: 2},
		{Name: "pokefarm_ui", Running: 1, Desired: 1, UpdateState: "rollback_completed"},
	}, nil)
	if c, _ := check(s, "service:pokefixer_fixer"); c.OK || c.Grace != 3 {
		t.Fatalf("fixer replicas: %+v", c)
	}
	if c, _ := check(s, "service:pokefarm_wall"); !c.OK {
		t.Fatalf("wall ok: %+v", c)
	}
	if c, _ := check(s, "rollback:pokefarm_ui"); c.OK {
		t.Fatalf("rollback: %+v", c)
	}
}

func TestEvaluateDiskAndMissingNodeReport(t *testing.T) {
	w := newWatcherForTest()
	now := time.Unix(1_800_000_000, 0)
	w.reports["n1"] = operatorapi.NodeReport{Node: "n1", At: now.Add(-time.Minute).Unix(), Disks: []operatorapi.DiskFree{{Mount: "/", FreeGB: 5}}}
	w.reports["n2"] = operatorapi.NodeReport{Node: "n2", At: now.Add(-20 * time.Minute).Unix()}
	s := w.evaluate(now, []operatorapi.SwarmNode{{Hostname: "n1", Status: "ready"}, {Hostname: "n2", Status: "ready"}, {Hostname: "n3", Status: "ready"}}, nil, nil)
	if c, _ := check(s, "disk:n1:/"); c.OK || !strings.Contains(c.Message, "5G") {
		t.Fatalf("disk: %+v", c)
	}
	if c, _ := check(s, "node-report:n2"); c.OK {
		t.Fatalf("stale report: %+v", c)
	}
	if c, ok := check(s, "node-report:n3"); !ok || c.OK {
		t.Fatalf("never-reported node must fail once watch has run 15m: %+v", c)
	}
}

func TestEvaluateFixerChecks(t *testing.T) {
	w := newWatcherForTest()
	now := time.Now()
	w.reports["f"] = operatorapi.NodeReport{Node: "f", At: now.Unix(), Fixer: &operatorapi.FixerSummary{PaidStarts24h: 20, PaidCap: 20, BlockedKeys: []string{"k1"}}}
	s := w.evaluate(now, []operatorapi.SwarmNode{{Hostname: "f", Status: "ready"}}, nil, nil)
	if c, _ := check(s, "paid-cap"); c.OK {
		t.Fatalf("paid cap: %+v", c)
	}
	if c, _ := check(s, "blocked-keys"); c.OK || !strings.Contains(c.Message, "k1") {
		t.Fatalf("blocked: %+v", c)
	}
	if s.Fixer == nil || s.Fixer.PaidStarts24h != 20 {
		t.Fatalf("snapshot fixer: %+v", s.Fixer)
	}
}

func TestFreezeSetsAndLiftsLabel(t *testing.T) {
	w := newWatcherForTest()
	now := time.Unix(1_800_000_000, 0)
	events := []rollbackEvent{{"a", now.Add(-3 * time.Hour)}, {"b", now.Add(-2 * time.Hour)}, {"c", now.Add(-time.Hour)}}
	until, changed := w.freezeDecision(now, events, 0)
	if !changed || until != now.Add(24*time.Hour).Unix() {
		t.Fatalf("freeze: %d %v", until, changed)
	}
	until, changed = w.freezeDecision(now.Add(25*time.Hour), nil, until)
	if !changed || until != 0 {
		t.Fatalf("lift: %d %v", until, changed)
	}
}

func TestNodeReportEndpointRequiresToken(t *testing.T) {
	w := newWatcherForTest()
	h := w.handler()
	body, _ := json.Marshal(operatorapi.NodeReport{Node: "n9"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/node-report", strings.NewReader(string(body))))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/node-report", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || w.reports["n9"].Node != "n9" {
		t.Fatalf("with token: %d %+v", rec.Code, w.reports)
	}
}

func TestDockerClientDecodesNodesAndServices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/nodes"):
			_, _ = rw.Write([]byte(`[{"Description":{"Hostname":"m1"},"Spec":{"Role":"manager"},"Status":{"State":"ready"},"ManagerStatus":{"Reachability":"reachable","Leader":true}}]`))
		case strings.HasSuffix(r.URL.Path, "/services"):
			_, _ = rw.Write([]byte(`[{"ID":"x","Version":{"Index":5},"Spec":{"Name":"pokefarm_wall","Labels":{"com.docker.stack.namespace":"pokefarm"}},"ServiceStatus":{"RunningTasks":1,"DesiredTasks":1},"UpdateStatus":{"State":"rollback_completed","CompletedAt":"2026-10-03T00:00:00Z"}}]`))
		}
	}))
	defer srv.Close()
	d := &dockerClient{http: srv.Client(), base: srv.URL}
	nodes, err := d.Nodes()
	if err != nil || len(nodes) != 1 || nodes[0].ManagerStatus != "leader" {
		t.Fatalf("nodes: %+v %v", nodes, err)
	}
	svcs, rolls, err := d.Services([]string{"pokefarm"})
	if err != nil || len(svcs) != 1 || svcs[0].Running != 1 || len(rolls) != 1 || rolls[0].Service != "pokefarm_wall" {
		t.Fatalf("services: %+v %+v %v", svcs, rolls, err)
	}
}
```

`newWatcherForTest()` lives in `watch_test.go`:

```go
func newWatcherForTest() *watcher {
	return &watcher{
		token: "test", minFreeGB: 20, freezeRollbacks: 3, freezeFor: 24 * time.Hour,
		reports: map[string]operatorapi.NodeReport{}, started: time.Unix(0, 0),
		mergedCount: -1, farmOpened: -1, farmClosed: -1,
	}
}
```

- [ ] **Step 2: Run** `go test ./cmd/pokewatch/ -count=1` → FAIL to compile.

- [ ] **Step 3: Implement**

`cmd/pokewatch/docker.go`:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

// dockerClient is the few Engine API calls the watcher needs, over the
// manager's unix socket. No SDK: three GETs and one service update.
type dockerClient struct {
	http *http.Client
	base string
}

func newDockerClient(socket string) *dockerClient {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &dockerClient{http: &http.Client{Transport: tr, Timeout: 30 * time.Second}, base: "http://docker/v1.43"}
}

type rollbackEvent struct {
	Service string
	At      time.Time
}

func (d *dockerClient) get(path string, out any) error {
	res, err := d.http.Get(d.base + path)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("docker GET %s: %s", path, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (d *dockerClient) Nodes() ([]operatorapi.SwarmNode, error) {
	var raw []struct {
		Description struct{ Hostname string }
		Spec        struct{ Role string }
		Status      struct{ State string }
		ManagerStatus *struct {
			Reachability string
			Leader       bool
		}
	}
	if err := d.get("/nodes", &raw); err != nil {
		return nil, err
	}
	out := make([]operatorapi.SwarmNode, 0, len(raw))
	for _, n := range raw {
		sn := operatorapi.SwarmNode{Hostname: n.Description.Hostname, Manager: n.Spec.Role == "manager", Status: n.Status.State}
		if n.ManagerStatus != nil {
			sn.ManagerStatus = n.ManagerStatus.Reachability
			if n.ManagerStatus.Leader {
				sn.ManagerStatus = "leader"
			}
		}
		out = append(out, sn)
	}
	return out, nil
}

// Services lists the stacks' services with replica counts and any rollback
// the Swarm performed (one event per service and completion time).
func (d *dockerClient) Services(stacks []string) ([]operatorapi.ServiceState, []rollbackEvent, error) {
	var raw []struct {
		ID      string
		Version struct{ Index uint64 }
		Spec    struct {
			Name   string
			Labels map[string]string
		}
		ServiceStatus *struct{ RunningTasks, DesiredTasks int }
		UpdateStatus  *struct {
			State       string
			CompletedAt time.Time
		}
	}
	if err := d.get("/services?status=true", &raw); err != nil {
		return nil, nil, err
	}
	want := map[string]bool{}
	for _, s := range stacks {
		want[s] = true
	}
	var out []operatorapi.ServiceState
	var rolls []rollbackEvent
	for _, s := range raw {
		if !want[s.Spec.Labels["com.docker.stack.namespace"]] {
			continue
		}
		st := operatorapi.ServiceState{Name: s.Spec.Name}
		if s.ServiceStatus != nil {
			st.Running, st.Desired = s.ServiceStatus.RunningTasks, s.ServiceStatus.DesiredTasks
		}
		if s.UpdateStatus != nil {
			st.UpdateState = s.UpdateStatus.State
			if strings.HasPrefix(s.UpdateStatus.State, "rollback") {
				rolls = append(rolls, rollbackEvent{Service: s.Spec.Name, At: s.UpdateStatus.CompletedAt})
			}
		}
		out = append(out, st)
	}
	return out, rolls, nil
}

// ServiceLabel reads one label of a service ("" when unset).
func (d *dockerClient) ServiceLabel(name, key string) (string, error) {
	var svc struct{ Spec struct{ Labels map[string]string } }
	if err := d.get("/services/"+url.PathEscape(name), &svc); err != nil {
		return "", err
	}
	return svc.Spec.Labels[key], nil
}

// SetServiceLabel sets (or with value "" removes) one service label. The
// spec is posted back unchanged otherwise, so tasks are not restarted:
// labels on the service spec do not touch the task template.
func (d *dockerClient) SetServiceLabel(name, key, value string) error {
	var svc struct {
		Version struct{ Index uint64 }
		Spec    map[string]any
	}
	if err := d.get("/services/"+url.PathEscape(name), &svc); err != nil {
		return err
	}
	labels, _ := svc.Spec["Labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
	}
	if value == "" {
		delete(labels, key)
	} else {
		labels[key] = value
	}
	svc.Spec["Labels"] = labels
	body, _ := json.Marshal(svc.Spec)
	res, err := d.http.Post(fmt.Sprintf("%s/services/%s/update?version=%d", d.base, url.PathEscape(name), svc.Version.Index), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("docker service update %s: %s", name, res.Status)
	}
	return nil
}
```

`cmd/pokewatch/github.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

type githubClient struct {
	http  *http.Client
	base  string
	token string
	repo  string
}

func (g *githubClient) search(q string, perPage int, out any) error {
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/search/issues?per_page=%d&q=%s", g.base, perPage, url.QueryEscape("repo:"+g.repo+" "+q)), nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	res, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("github search: %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Count returns total_count for a search, or -1 on error.
func (g *githubClient) Count(q string) int {
	var out struct {
		Total int `json:"total_count"`
	}
	if err := g.search(q, 1, &out); err != nil {
		return -1
	}
	return out.Total
}

// TriagePRs lists open "[triage:" pull requests.
func (g *githubClient) TriagePRs() ([]operatorapi.PullRequest, error) {
	var out struct {
		Items []struct {
			Number    int       `json:"number"`
			Title     string    `json:"title"`
			HTMLURL   string    `json:"html_url"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := g.search(`is:pr is:open in:title "[triage:"`, 50, &out); err != nil {
		return nil, err
	}
	prs := make([]operatorapi.PullRequest, 0, len(out.Items))
	for _, it := range out.Items {
		prs = append(prs, operatorapi.PullRequest{Number: it.Number, Title: it.Title, URL: it.HTMLURL, CreatedAt: it.CreatedAt.Unix()})
	}
	return prs, nil
}
```

`cmd/pokewatch/watch.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

const frozenLabel = "pokepilot.deploy-frozen-until"

type watcher struct {
	token           string
	minFreeGB       int
	freezeRollbacks int
	freezeFor       time.Duration
	freezeService   string
	stacks          []string
	docker          *dockerClient
	gh              *githubClient
	pushURL         string
	started         time.Time

	mu      sync.Mutex
	reports map[string]operatorapi.NodeReport
	seen    map[string]time.Time // rollback service|completedAt → first seen

	// GitHub numbers refresh every 15 min (search API is rate limited).
	ghAt        time.Time
	triagePRs   []operatorapi.PullRequest
	ghErr       bool
	mergedCount int
	farmOpened  int
	farmClosed  int
}

func runWatch(ctx context.Context, token string) {
	w := &watcher{
		token:           token,
		minFreeGB:       envInt("POKEPILOT_WATCH_MIN_FREE_GB", 20),
		freezeRollbacks: envInt("POKEPILOT_FREEZE_ROLLBACKS", 3),
		freezeFor:       time.Duration(envInt("POKEPILOT_FREEZE_SECONDS", 86400)) * time.Second,
		freezeService:   env("POKEWATCH_FREEZE_SERVICE", "pokefarm_wall"),
		stacks:          strings.Split(env("POKEWATCH_STACKS", "pokefarm,pokefixer"), ","),
		docker:          newDockerClient(env("DOCKER_SOCKET", "/var/run/docker.sock")),
		gh: &githubClient{http: &http.Client{Timeout: 20 * time.Second}, base: "https://api.github.com",
			token: operatorapi.ReadSecretFile(env("POKEPILOT_GITHUB_TOKEN_FILE", "")), repo: env("POKEPILOT_GITHUB_REPO", "maestroi/PokePilot")},
		pushURL: env("POKEPILOT_TELEGRAM_URL", "http://telegram:8080") + "/v1/ops",
		started: time.Now(),
		reports: map[string]operatorapi.NodeReport{},
		seen:    map[string]time.Time{},
		mergedCount: -1, farmOpened: -1, farmClosed: -1,
	}
	srv := &http.Server{Addr: ":8080", Handler: w.handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { log.Printf("pokewatch: %v", srv.ListenAndServe()) }()
	client := &http.Client{Timeout: 20 * time.Second}
	tick := time.NewTicker(envDuration("POKEWATCH_INTERVAL", time.Minute))
	defer tick.Stop()
	for {
		snap := w.tick(time.Now())
		if err := operatorapi.PostOps(ctx, client, w.pushURL, token, snap); err != nil {
			log.Printf("pokewatch: push: %v", err)
		}
		select {
		case <-ctx.Done():
			_ = srv.Close()
			return
		case <-tick.C:
		}
	}
}

func (w *watcher) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /v1/node-report", func(rw http.ResponseWriter, r *http.Request) {
		if !operatorapi.OpsAuthorized(r, w.token) {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		var rep operatorapi.NodeReport
		if err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 64<<10)).Decode(&rep); err != nil || rep.Node == "" {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		rep.At = time.Now().Unix() // trust our clock, not the node's
		w.mu.Lock()
		w.reports[rep.Node] = rep
		w.mu.Unlock()
	})
	return mux
}

// tick gathers live state, applies the freeze, and evaluates every check.
// A failing source only fails its own check.
func (w *watcher) tick(now time.Time) operatorapi.OpsSnapshot {
	var extra []operatorapi.CheckResult
	nodes, err := w.docker.Nodes()
	if err != nil {
		extra = append(extra, operatorapi.CheckResult{Name: "docker-api", Grace: 3, Message: "watcher cannot read Swarm state: " + err.Error()})
	}
	services, rolls, err := w.docker.Services(w.stacks)
	if err != nil {
		extra = append(extra, operatorapi.CheckResult{Name: "docker-api", Grace: 3, Message: "watcher cannot list services: " + err.Error()})
	}
	events := w.recordRollbacks(rolls, now)
	frozen := int64(0)
	if v, err := w.docker.ServiceLabel(w.freezeService, frozenLabel); err == nil {
		frozen, _ = strconv.ParseInt(v, 10, 64)
	}
	if until, changed := w.freezeDecision(now, events, frozen); changed {
		value := ""
		if until > 0 {
			value = strconv.FormatInt(until, 10)
		}
		if err := w.docker.SetServiceLabel(w.freezeService, frozenLabel, value); err != nil {
			log.Printf("pokewatch: freeze label: %v", err)
		} else {
			frozen = until
		}
	}
	w.refreshGitHub(now)
	snap := w.evaluate(now, nodes, services, events)
	snap.FrozenUntil = frozen
	if frozen > now.Unix() {
		snap.Checks = append(snap.Checks, operatorapi.CheckResult{Name: "deploy-frozen", Grace: 1,
			Message: fmt.Sprintf("deploys frozen until %s after %d+ rollbacks; runs unaffected", time.Unix(frozen, 0).UTC().Format("Jan 2 15:04 UTC"), w.freezeRollbacks)})
	} else {
		snap.Checks = append(snap.Checks, operatorapi.CheckResult{Name: "deploy-frozen", OK: true, Grace: 1})
	}
	if len(extra) > 0 {
		snap.Checks = append(snap.Checks, extra[0])
	} else {
		snap.Checks = append(snap.Checks, operatorapi.CheckResult{Name: "docker-api", OK: true, Grace: 3})
	}
	return snap
}

// recordRollbacks remembers each (service, completion) once and returns the
// events inside the freeze window.
func (w *watcher) recordRollbacks(rolls []rollbackEvent, now time.Time) []rollbackEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, r := range rolls {
		key := r.Service + "|" + r.At.UTC().Format(time.RFC3339)
		if _, ok := w.seen[key]; !ok {
			w.seen[key] = r.At
		}
	}
	var out []rollbackEvent
	for key, at := range w.seen {
		if now.Sub(at) > w.freezeFor {
			delete(w.seen, key)
			continue
		}
		out = append(out, rollbackEvent{Service: strings.SplitN(key, "|", 2)[0], At: at})
	}
	return out
}

// freezeDecision returns the new frozen-until value and whether it changed.
func (w *watcher) freezeDecision(now time.Time, events []rollbackEvent, frozenUntil int64) (int64, bool) {
	if frozenUntil > 0 && now.Unix() >= frozenUntil {
		return 0, true
	}
	if frozenUntil == 0 && len(events) >= w.freezeRollbacks {
		return now.Add(w.freezeFor).Unix(), true
	}
	return frozenUntil, false
}

func (w *watcher) refreshGitHub(now time.Time) {
	if w.gh == nil || now.Sub(w.ghAt) < 15*time.Minute {
		return
	}
	w.ghAt = now
	prs, err := w.gh.TriagePRs()
	w.ghErr = err != nil
	if err == nil {
		w.triagePRs = prs
	}
	since := now.Add(-24 * time.Hour).UTC().Format("2006-01-02T15:04:05Z")
	w.mergedCount = w.gh.Count("is:pr is:merged merged:>=" + since)
	w.farmOpened = w.gh.Count(`is:issue in:title "[farm]" created:>=` + since)
	w.farmClosed = w.gh.Count(`is:issue is:closed in:title "[farm]" closed:>=` + since)
}

// evaluate turns gathered state into checks and the snapshot the bot renders.
func (w *watcher) evaluate(now time.Time, nodes []operatorapi.SwarmNode, services []operatorapi.ServiceState, events []rollbackEvent) operatorapi.OpsSnapshot {
	s := operatorapi.OpsSnapshot{At: now.Unix(), Nodes: nodes, Services: services,
		TriagePRs: w.triagePRs, Merged24h: w.mergedCount, FarmOpened: w.farmOpened, FarmClosed: w.farmClosed}
	add := func(name string, grace int, bad bool, msg string) {
		c := operatorapi.CheckResult{Name: name, Grace: grace, OK: !bad}
		if bad {
			c.Message = msg
		}
		s.Checks = append(s.Checks, c)
	}

	managers, reachable := 0, 0
	for _, n := range nodes {
		down := n.Status != "ready"
		unreach := n.Manager && n.ManagerStatus == "unreachable"
		msg := fmt.Sprintf("node %s is %s", n.Hostname, n.Status)
		if unreach {
			msg = fmt.Sprintf("manager %s is unreachable", n.Hostname)
		}
		add("node:"+n.Hostname, 2, down || unreach, msg)
		if n.Manager {
			managers++
			if !down && !unreach {
				reachable++
			}
		}
	}
	if managers > 0 {
		add("quorum", 2, reachable < managers,
			fmt.Sprintf("%d/%d managers reachable; %d more failure(s) and Swarm cannot schedule", reachable, managers, reachable-(managers/2+1)+1))
	}

	for _, svc := range services {
		add("service:"+svc.Name, 3, svc.Running < svc.Desired, fmt.Sprintf("%s at %d/%d replicas", svc.Name, svc.Running, svc.Desired))
		add("rollback:"+svc.Name, 1, strings.HasPrefix(svc.UpdateState, "rollback"),
			fmt.Sprintf("Swarm rolled %s back from a crash-looping image (rollout holds until the next merge)", svc.Name))
	}

	w.mu.Lock()
	reports := make([]operatorapi.NodeReport, 0, len(w.reports))
	for _, r := range w.reports {
		reports = append(reports, r)
	}
	w.mu.Unlock()
	sort.Slice(reports, func(i, j int) bool { return reports[i].Node < reports[j].Node })
	s.Disks = reports
	byNode := map[string]operatorapi.NodeReport{}
	for _, r := range reports {
		byNode[r.Node] = r
		for _, d := range r.Disks {
			add("disk:"+r.Node+":"+d.Mount, 2, d.FreeGB < w.minFreeGB,
				fmt.Sprintf("%s %s has %dG free (under %dG)", r.Node, d.Mount, d.FreeGB, w.minFreeGB))
		}
		if r.Fixer != nil {
			f := *r.Fixer
			s.Fixer = &f
		}
	}
	warm := now.Sub(w.started) >= 15*time.Minute
	for _, n := range nodes {
		if n.Status != "ready" {
			continue // node:<host> already pages
		}
		r, ok := byNode[n.Hostname]
		stale := (ok && now.Unix()-r.At > 15*60) || (!ok && warm)
		add("node-report:"+n.Hostname, 1, stale, fmt.Sprintf("no node report from %s for 15m (pokefarm-ops_node task down?)", n.Hostname))
	}

	if s.Fixer != nil {
		add("paid-cap", 1, s.Fixer.PaidStarts24h >= s.Fixer.PaidCap,
			fmt.Sprintf("paid fixer cap reached (%d/%d starts in 24h); only qwen runs until it ages out", s.Fixer.PaidStarts24h, s.Fixer.PaidCap))
		add("blocked-keys", 1, len(s.Fixer.BlockedKeys) > 0,
			"fixer stopped on triage keys (all tiers failed or verdict parked): "+strings.Join(s.Fixer.BlockedKeys, ", "))
	}

	var stuck []string
	for _, pr := range w.triagePRs {
		if now.Unix()-pr.CreatedAt > 12*3600 {
			stuck = append(stuck, fmt.Sprintf("#%d", pr.Number))
		}
	}
	add("stuck-prs", 1, len(stuck) > 0, "fixer PRs open over 12h (CI failing?): "+strings.Join(stuck, ", "))
	_ = events
	return s
}
```

Note on the quorum message arithmetic: simplify while implementing if it reads wrong for a case; the test only pins `2/3`. A clearer message is `fmt.Sprintf("%d/%d managers reachable", reachable, managers)`; use that if the "more failure(s)" count is off for even manager counts.

- [ ] **Step 4: Run** `go test ./cmd/pokewatch/ -count=1` → PASS. Also `go vet ./cmd/pokewatch/`.

- [ ] **Step 5: Commit**

```bash
git add cmd/pokewatch/
git commit -m "feat(pokewatch): Swarm, GitHub and fixer checks with label-based deploy freeze"
```

---

### Task 5: Rollout honors the freeze label

**Files:**
- Modify: `deploy/rollout-latest.sh` (after the "stack not deployed" guard, around line 24)
- Test: `deploy/pull_latest_test.go` (add one test)

**Interfaces:**
- Consumes: service label `pokepilot.deploy-frozen-until=<unix>` on `${STACK}_wall` (Task 4).

- [ ] **Step 1: Write the failing test** (append to `deploy/pull_latest_test.go`)

```go
func TestRolloutLatestSkipsWhileDeploysAreFrozen(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	until := time.Now().Add(time.Hour).Unix()
	mockDocker := fmt.Sprintf(`#!/usr/bin/env bash
set -eu
printf '%%s\n' "$*" >> "$MOCK_DOCKER_LOG"
if [ "$1" = "service" ] && [ "$2" = "inspect" ]; then
	case "$*" in
	*deploy-frozen-until*) echo '%d' ;;
	esac
	exit 0
fi
exit 0
`, until)
	if err := os.WriteFile(filepath.Join(tmp, "docker"), []byte(mockDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "./rollout-latest.sh")
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MOCK_DOCKER_LOG="+logPath,
		"FARM_IMAGE_DIGEST_REF=ghcr.io/maestroi/pokepilot@sha256:new",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rollout-latest.sh: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "deploys frozen until") {
		t.Fatalf("missing freeze message:\n%s", out)
	}
	logData, _ := os.ReadFile(logPath)
	if strings.Contains(string(logData), "service update") {
		t.Fatalf("frozen rollout must not update services:\n%s", logData)
	}
}
```

(add `"fmt"` and `"time"` to the test file's imports if absent.)

- [ ] **Step 2: Run** `go test ./deploy/ -run TestRolloutLatestSkipsWhileDeploysAreFrozen -count=1` → FAIL (no freeze message; services updated).

- [ ] **Step 3: Implement** — insert after the `stack ${STACK} not deployed; skip` block:

```bash
# pokewatch (deploy/ops.yml) freezes deploys after repeated Swarm rollbacks by
# labeling the wall service. Runs keep playing on the current image; the
# freeze lifts by itself at the timestamp.
frozen_until=$(docker service inspect "${STACK}_wall" --format '{{index .Spec.Labels "pokepilot.deploy-frozen-until"}}' 2>/dev/null || true)
if [[ "$frozen_until" =~ ^[0-9]+$ ]] && [ "$frozen_until" -gt "$(date +%s)" ]; then
	echo "pokefarm-pull: deploys frozen until $(date -u -d "@$frozen_until" '+%F %T UTC') after repeated rollbacks; skip"
	exit 0
fi
```

- [ ] **Step 4: Run** `go test ./deploy/ -run 'Rollout|PullLatest' -count=1` → PASS (new and existing rollout tests; existing mocks print nothing for the label, so they are unaffected).

- [ ] **Step 5: Commit**

```bash
git add deploy/rollout-latest.sh deploy/pull_latest_test.go
git commit -m "feat(deploy): rollout skips while pokewatch has frozen deploys"
```

---

### Task 6: Bot alert state machine

**Files:**
- Create: `cmd/poketelegram/alerts.go`, `cmd/poketelegram/alerts_test.go`

**Interfaces:**
- Consumes: `operatorapi.CheckResult`.
- Produces:

```go
type alertKind int
const (alertOpen alertKind = iota + 1; alertRemind; alertResolve)

type alertState struct {
	Name, Source, Message, RunID, Link string
	Bad        int
	Open       bool
	Since      time.Time // first bad observation of the open episode
	Notified   time.Time
	MutedUntil time.Time
	Cards      map[int64]int64 // chat → alert card message id
}

type alertAction struct {
	Kind  alertKind
	State *alertState // live pointer; renderer may set Cards
}

type alertBook struct {
	remind time.Duration
	states map[string]*alertState
	seeded map[string]bool
}

func newAlertBook(remind time.Duration) *alertBook
// Observe applies one full result list from source. Results previously seen
// from the same source but absent now count as OK. The first call per source
// adopts bad results as open without paging and returns them as `adopted`.
func (a *alertBook) Observe(source string, results []operatorapi.CheckResult, now time.Time) (actions []alertAction, adopted []*alertState)
func (a *alertBook) Mute(name string, until time.Time) bool
func (a *alertBook) Open() []*alertState // open alerts sorted by Since
```

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"testing"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func bad(name string, grace int) operatorapi.CheckResult {
	return operatorapi.CheckResult{Name: name, Grace: grace, Message: name + " broke"}
}
func ok(name string) operatorapi.CheckResult { return operatorapi.CheckResult{Name: name, OK: true} }

func kinds(actions []alertAction) []alertKind {
	out := []alertKind{}
	for _, a := range actions {
		out = append(out, a.Kind)
	}
	return out
}

func TestAlertGraceRemindResolve(t *testing.T) {
	a := newAlertBook(12 * time.Hour)
	t0 := time.Unix(1_800_000_000, 0)
	a.Observe("watch", []operatorapi.CheckResult{ok("disk")}, t0) // seed
	if acts, _ := a.Observe("watch", []operatorapi.CheckResult{bad("disk", 2)}, t0.Add(time.Minute)); len(acts) != 0 {
		t.Fatalf("inside grace: %v", kinds(acts))
	}
	acts, _ := a.Observe("watch", []operatorapi.CheckResult{bad("disk", 2)}, t0.Add(2*time.Minute))
	if len(acts) != 1 || acts[0].Kind != alertOpen {
		t.Fatalf("open: %v", kinds(acts))
	}
	if acts, _ = a.Observe("watch", []operatorapi.CheckResult{bad("disk", 2)}, t0.Add(3*time.Hour)); len(acts) != 0 {
		t.Fatalf("no reminder before 12h: %v", kinds(acts))
	}
	if acts, _ = a.Observe("watch", []operatorapi.CheckResult{bad("disk", 2)}, t0.Add(13*time.Hour)); len(acts) != 1 || acts[0].Kind != alertRemind {
		t.Fatalf("remind: %v", kinds(acts))
	}
	acts, _ = a.Observe("watch", []operatorapi.CheckResult{ok("disk")}, t0.Add(14*time.Hour))
	if len(acts) != 1 || acts[0].Kind != alertResolve || acts[0].State.Since != t0.Add(2*time.Minute) {
		t.Fatalf("resolve: %+v", acts)
	}
}

func TestAlertMissingResultFromSameSourceResolves(t *testing.T) {
	a := newAlertBook(12 * time.Hour)
	now := time.Now()
	a.Observe("bot", nil, now)
	a.Observe("bot", []operatorapi.CheckResult{bad("stall:r1", 1)}, now)
	acts, _ := a.Observe("bot", nil, now.Add(time.Minute))
	if len(acts) != 1 || acts[0].Kind != alertResolve {
		t.Fatalf("run gone → resolve: %v", kinds(acts))
	}
	// another source's list never resolves this source's checks
	a.Observe("bot", []operatorapi.CheckResult{bad("wall", 1)}, now)
	if acts, _ := a.Observe("watch", nil, now); len(acts) != 0 {
		t.Fatalf("cross-source resolve: %v", kinds(acts))
	}
}

func TestAlertRestartAdoptsWithoutPaging(t *testing.T) {
	a := newAlertBook(12 * time.Hour)
	acts, adopted := a.Observe("watch", []operatorapi.CheckResult{bad("quorum", 2), ok("disk")}, time.Now())
	if len(acts) != 0 || len(adopted) != 1 || !adopted[0].Open {
		t.Fatalf("restart: acts=%v adopted=%v", kinds(acts), adopted)
	}
	acts, _ = a.Observe("watch", []operatorapi.CheckResult{ok("quorum")}, time.Now())
	if len(acts) != 1 || acts[0].Kind != alertResolve {
		t.Fatalf("adopted alert must still resolve: %v", kinds(acts))
	}
}

func TestAlertMuteSuppressesOpenAndRemind(t *testing.T) {
	a := newAlertBook(12 * time.Hour)
	t0 := time.Now()
	a.Observe("watch", nil, t0)
	a.Observe("watch", []operatorapi.CheckResult{bad("paid-cap", 1)}, t0)
	if !a.Mute("paid-cap", t0.Add(12*time.Hour)) {
		t.Fatal("mute open alert")
	}
	if acts, _ := a.Observe("watch", []operatorapi.CheckResult{bad("paid-cap", 1)}, t0.Add(11*time.Hour+59*time.Minute)); len(acts) != 0 {
		t.Fatalf("muted: %v", kinds(acts))
	}
	if acts, _ := a.Observe("watch", []operatorapi.CheckResult{bad("paid-cap", 1)}, t0.Add(13*time.Hour)); len(acts) != 1 || acts[0].Kind != alertRemind {
		t.Fatalf("mute expired → remind: %v", kinds(acts))
	}
}
```

- [ ] **Step 2: Run** `go test ./cmd/poketelegram/ -run Alert -count=1` → FAIL to compile.

- [ ] **Step 3: Implement** `cmd/poketelegram/alerts.go`:

```go
package main

import (
	"sort"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

// (types from the Interfaces block above)

func newAlertBook(remind time.Duration) *alertBook {
	return &alertBook{remind: remind, states: map[string]*alertState{}, seeded: map[string]bool{}}
}

func (a *alertBook) Observe(source string, results []operatorapi.CheckResult, now time.Time) ([]alertAction, []*alertState) {
	var actions []alertAction
	var adopted []*alertState
	first := !a.seeded[source]
	a.seeded[source] = true
	present := map[string]bool{}
	for _, r := range results {
		present[r.Name] = true
		st := a.states[r.Name]
		if st == nil {
			st = &alertState{Name: r.Name, Source: source, Cards: map[int64]int64{}}
			a.states[r.Name] = st
		}
		if r.OK {
			if st.Open {
				st.Open = false
				actions = append(actions, alertAction{Kind: alertResolve, State: st})
			}
			st.Bad = 0
			continue
		}
		st.Bad++
		st.Message, st.RunID, st.Link = r.Message, r.RunID, r.Link
		if st.Bad == 1 {
			st.Since = now
		}
		grace := r.Grace
		if grace < 1 {
			grace = 1
		}
		switch {
		case !st.Open && first:
			st.Open, st.Notified = true, now
			adopted = append(adopted, st)
		case !st.Open && st.Bad >= grace:
			st.Open = true
			if now.After(st.MutedUntil) {
				st.Notified = now
				actions = append(actions, alertAction{Kind: alertOpen, State: st})
			}
		case st.Open && now.After(st.MutedUntil) && now.Sub(st.Notified) >= a.remind:
			st.Notified = now
			actions = append(actions, alertAction{Kind: alertRemind, State: st})
		}
	}
	for name, st := range a.states {
		if st.Source != source || present[name] {
			continue
		}
		if st.Open {
			actions = append(actions, alertAction{Kind: alertResolve, State: st})
		}
		delete(a.states, name)
	}
	return actions, adopted
}

func (a *alertBook) Mute(name string, until time.Time) bool {
	st := a.states[name]
	if st == nil || !st.Open {
		return false
	}
	st.MutedUntil = until
	// The reminder falls due the moment the mute expires.
	st.Notified = until.Add(-a.remind)
	return true
}

func (a *alertBook) Open() []*alertState {
	var out []*alertState
	for _, st := range a.states {
		if st.Open {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}
```


- [ ] **Step 4: Run** `go test ./cmd/poketelegram/ -run Alert -count=1` → PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/poketelegram/alerts.go cmd/poketelegram/alerts_test.go
git commit -m "feat(poketelegram): alert state machine with grace, reminders, mute and restart adoption"
```

---

### Task 7: Telegram client upgrade (HTML, edit, reply, commands)

**Files:**
- Create: `cmd/poketelegram/telegram.go`, `cmd/poketelegram/telegram_test.go`
- Modify: `cmd/poketelegram/main.go` (move `telegramClient` methods and Telegram types out; keep behavior)

**Interfaces:**
- Produces:

```go
type outgoing struct {
	Text     string
	HTML     bool
	Keyboard *inlineKeyboard
	ReplyTo  int64
	Silent   bool
}
func (t *telegramClient) send(ctx context.Context, chatID int64, m outgoing) (int64, error) // returns message_id
func (t *telegramClient) edit(ctx context.Context, chatID, messageID int64, m outgoing) error
func (t *telegramClient) setCommands(ctx context.Context, cmds [][2]string) error
var errMessageGone = errors.New("telegram: message cannot be edited")
var errNotModified = errors.New("telegram: message is not modified")
func h(s string) string // html.EscapeString
```

- `telegramClient` gains `base string` (empty → `https://api.telegram.org`), used by `endpoint`.
- `telegramMessage` gains `ReplyToMessage *telegramMessage \`json:"reply_to_message,omitempty"\``.
- `sendMessage(ctx, chat, text, keyboard) error` stays as a thin wrapper over `send` (plain text) so existing call sites compile.

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeTG struct {
	srv   *httptest.Server
	calls []map[string]any
	reply func(method string, body map[string]any) (int, string)
}

func newFakeTG(t *testing.T) *fakeTG {
	f := &fakeTG{}
	f.reply = func(string, map[string]any) (int, string) { return 200, `{"ok":true,"result":{"message_id":42}}` }
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		body := map[string]any{"_method": method}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.calls = append(f.calls, body)
		code, out := f.reply(method, body)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTG) client() *telegramClient {
	return &telegramClient{token: "T", http: f.srv.Client(), base: f.srv.URL}
}

func TestSendHTMLReturnsMessageIDAndReplies(t *testing.T) {
	f := newFakeTG(t)
	id, err := f.client().send(context.Background(), 7, outgoing{Text: "<b>" + h("a<b&c") + "</b>", HTML: true, ReplyTo: 9, Silent: true})
	if err != nil || id != 42 {
		t.Fatalf("send: %d %v", id, err)
	}
	c := f.calls[0]
	if c["parse_mode"] != "HTML" || c["text"] != "<b>a&lt;b&amp;c</b>" || c["disable_notification"] != true {
		t.Fatalf("payload: %+v", c)
	}
	if rp, _ := c["reply_parameters"].(map[string]any); rp["message_id"] != float64(9) || rp["allow_sending_without_reply"] != true {
		t.Fatalf("reply: %+v", c)
	}
}

func TestEditClassifiesTelegramErrors(t *testing.T) {
	f := newFakeTG(t)
	f.reply = func(string, map[string]any) (int, string) {
		return 400, `{"ok":false,"description":"Bad Request: message is not modified"}`
	}
	if err := f.client().edit(context.Background(), 1, 2, outgoing{Text: "x"}); !errors.Is(err, errNotModified) {
		t.Fatalf("not modified: %v", err)
	}
	f.reply = func(string, map[string]any) (int, string) {
		return 400, `{"ok":false,"description":"Bad Request: message to edit not found"}`
	}
	if err := f.client().edit(context.Background(), 1, 2, outgoing{Text: "x"}); !errors.Is(err, errMessageGone) {
		t.Fatalf("gone: %v", err)
	}
	f.reply = func(string, map[string]any) (int, string) {
		return 400, `{"ok":false,"description":"Bad Request: message can't be edited"}`
	}
	if err := f.client().edit(context.Background(), 1, 2, outgoing{Text: "x"}); !errors.Is(err, errMessageGone) {
		t.Fatalf("too old: %v", err)
	}
}

func TestSetCommands(t *testing.T) {
	f := newFakeTG(t)
	f.reply = func(string, map[string]any) (int, string) { return 200, `{"ok":true,"result":true}` }
	if err := f.client().setCommands(context.Background(), [][2]string{{"menu", "Open the menu"}}); err != nil {
		t.Fatal(err)
	}
	if f.calls[0]["_method"] != "setMyCommands" {
		t.Fatalf("%+v", f.calls)
	}
}
```

- [ ] **Step 2: Run** `go test ./cmd/poketelegram/ -run 'SendHTML|EditClassifies|SetCommands' -count=1` → FAIL to compile.

- [ ] **Step 3: Implement**

Move from `main.go` to `telegram.go`: Telegram types (`telegramUser` … `inlineButton`), `telegramClient` and all its methods, unchanged except:

```go
type telegramClient struct {
	token string
	http  *http.Client
	base  string
}

func (t *telegramClient) endpoint(method string) string {
	base := t.base
	if base == "" {
		base = "https://api.telegram.org"
	}
	return base + "/bot" + t.token + "/" + method
}
```

`do` currently fails on non-2xx before decoding; change it to decode the Telegram error body so `description` is available:

```go
func (t *telegramClient) do(req *http.Request, out any) error {
	res, err := t.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var e struct {
			Description string `json:"description"`
		}
		_ = json.Unmarshal(data, &e)
		return classifyTelegramError(fmt.Errorf("telegram API %s: %s", res.Status, firstNonEmpty(e.Description, strings.TrimSpace(string(data)))))
	}
	return json.Unmarshal(data, out)
}

var (
	errMessageGone = errors.New("telegram: message cannot be edited")
	errNotModified = errors.New("telegram: message is not modified")
)

func classifyTelegramError(err error) error {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "message is not modified"):
		return fmt.Errorf("%w: %v", errNotModified, err)
	case strings.Contains(msg, "message to edit not found"), strings.Contains(msg, "message can't be edited"):
		return fmt.Errorf("%w: %v", errMessageGone, err)
	}
	return err
}

func h(s string) string { return html.EscapeString(s) }

type outgoing struct {
	Text     string
	HTML     bool
	Keyboard *inlineKeyboard
	ReplyTo  int64
	Silent   bool
}

func (m outgoing) payload(chatID int64) map[string]any {
	p := map[string]any{
		"chat_id":              chatID,
		"text":                 clip(m.Text, 3900),
		"link_preview_options": map[string]any{"is_disabled": true},
	}
	if m.HTML {
		p["parse_mode"] = "HTML"
	}
	if m.Keyboard != nil {
		p["reply_markup"] = m.Keyboard
	}
	if m.ReplyTo != 0 {
		p["reply_parameters"] = map[string]any{"message_id": m.ReplyTo, "allow_sending_without_reply": true}
	}
	if m.Silent {
		p["disable_notification"] = true
	}
	return p
}

func (t *telegramClient) send(ctx context.Context, chatID int64, m outgoing) (int64, error) {
	var out telegramResponse[telegramMessage]
	if err := t.callJSON(ctx, "sendMessage", m.payload(chatID), &out); err != nil {
		return 0, err
	}
	if !out.OK {
		return 0, errors.New(out.Description)
	}
	return out.Result.MessageID, nil
}

func (t *telegramClient) edit(ctx context.Context, chatID, messageID int64, m outgoing) error {
	p := m.payload(chatID)
	p["message_id"] = messageID
	delete(p, "reply_parameters")
	delete(p, "disable_notification")
	var out telegramResponse[json.RawMessage]
	return t.callJSON(ctx, "editMessageText", p, &out)
}

func (t *telegramClient) setCommands(ctx context.Context, cmds [][2]string) error {
	list := make([]map[string]string, 0, len(cmds))
	for _, c := range cmds {
		list = append(list, map[string]string{"command": c[0], "description": c[1]})
	}
	var out telegramResponse[bool]
	return t.callJSON(ctx, "setMyCommands", map[string]any{"commands": list}, &out)
}

func (t *telegramClient) sendMessage(ctx context.Context, chatID int64, text string, keyboard *inlineKeyboard) error {
	_, err := t.send(ctx, chatID, outgoing{Text: text, Keyboard: keyboard})
	return err
}
```

(`clip` must not cut an HTML entity or tag in half: change `clip` callers for HTML text to clip the *unescaped* pieces before escaping; the 3900-rune clip in `payload` is a last-resort guard. If a test shows a cut tag, make `payload` clip only when `!m.HTML`, and keep card renderers under the limit by construction — Task 8 renderers cap list lengths.)

- [ ] **Step 4: Run** `go test ./cmd/poketelegram/ -count=1` → PASS (all existing bot tests still pass after the move).

- [ ] **Step 5: Commit**

```bash
git add cmd/poketelegram/
git commit -m "refactor(poketelegram): Telegram client in its own file with HTML, edit, reply and commands"
```

---

### Task 8: Cards, navigation, numbers, reply shortcuts, flag flow

**Files:**
- Create: `cmd/poketelegram/ui.go`, `cmd/poketelegram/ui_test.go`
- Modify: `cmd/poketelegram/main.go` (`handleMessage`, `handleCallback`, `bot` struct, `observeRuns` sampling, `helpText`, `main` → `setCommands`)

**Interfaces:**
- Consumes: Task 7 `send`/`edit`/`h`; Task 1 `op.FlagStuck`; Task 9 `b.ops` snapshot (Health/Fixer cards read `b.opsSnapshot()`; until Task 9 lands, a nil snapshot renders "No watcher data yet.").
- Produces:

```go
type card struct {
	Text     string // HTML
	Keyboard *inlineKeyboard
	RunID    string // set when the card is about one run (reply shortcuts)
}
func (b *bot) homeCard() card
func (b *bot) runsCard(ctx context.Context, chat int64, page int) (card, error)
func (b *bot) runCard(ctx context.Context, runID string) (card, error)
func (b *bot) failuresCard(ctx context.Context, page int) (card, error)
func (b *bot) healthCard() card
func (b *bot) fixerCard() card
func (b *bot) alertsCard(ctx context.Context) (card, error)
func (b *bot) handle(value string) string       // value → 10-char handle, remembered
func (b *bot) unhandle(handle string) string    // handle → value ("" unknown)
func (b *bot) resolveRun(chat int64, arg string) string // "2" → run from last list; else arg
func (b *bot) rememberRunMessage(chat, msgID int64, runID string)
func badgeBar(n, total int) string
type runSample struct{ At time.Time; Frame uint64; Maps int }
```

- Callback data grammar (all ≤ 64 bytes): `nav:<home|runs|failures|health|fixer|alerts>`, `pg:runs:<n>`, `pg:fail:<n>`, `run:<h>`, `act:<frame|replay|triage|flag|stop|restart>:<h>`, `ref:<card>:<h?>`, `mute:<h>`, `confirm:<token>`, `cancel:<token>`.
- New `bot` fields: `handles map[string]string`, `lastList map[int64][]string`, `msgRuns map[int64]map[int64]string`, `samples map[string][]runSample`, `lastNewMap map[string]time.Time`.

- [ ] **Step 1: Write the failing tests** (`cmd/poketelegram/ui_test.go`)

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func uiBot(t *testing.T, runs []operatorapi.Run) (*bot, *fakeTG) {
	t.Helper()
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/dashboard"):
			_ = json.NewEncoder(w).Encode(map[string]any{"runs": runs})
		case strings.HasSuffix(r.URL.Path, "/flag-stuck"):
			_ = json.NewEncoder(w).Encode(map[string]any{"flagged": true})
		case strings.HasPrefix(r.URL.Path, "/v1/runs/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/runs/")
			for _, run := range runs {
				if run.RunID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"run": run})
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/v1/triage"):
			_, _ = w.Write([]byte(`[]`)) // GET /v1/triage returns a bare array
		}
	}))
	t.Cleanup(wall.Close)
	f := newFakeTG(t)
	b := newBot(config{
		AllowedUsers: map[int64]struct{}{1: {}}, AllowedChats: map[int64]struct{}{},
		AdminBaseURL: "https://admin", SpectatorBase: "https://spec", ControlSpacing: 0, StallAfter: 15 * time.Minute,
	}, f.client(), operatorapi.New(wall.URL, "", ""))
	return b, f
}

func TestRunsCardIsTappableAndNumbered(t *testing.T) {
	runs := []operatorapi.Run{
		{RunID: "red-very-long-run-id-0001-abcdef", Status: "running", Game: "red", Player: &operatorapi.Player{Badges: []string{"a", "b"}}, GameState: map[string]any{"map_name": "Route <12>"}},
		{RunID: "gold-0002", Status: "running", Game: "gold", Map: 7},
	}
	b, _ := uiBot(t, runs)
	c, err := b.runsCard(context.Background(), 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Text, "Route &lt;12&gt;") || !strings.Contains(c.Text, "map 7") {
		t.Fatalf("labels/escaping: %s", c.Text)
	}
	for _, row := range c.Keyboard.InlineKeyboard {
		for _, btn := range row {
			if len(btn.CallbackData) > 64 {
				t.Fatalf("callback too long: %q", btn.CallbackData)
			}
		}
	}
	if b.resolveRun(5, "2") != "gold-0002" || b.resolveRun(5, "gold-0002") != "gold-0002" || b.resolveRun(6, "2") != "2" {
		t.Fatal("number resolution is per chat and falls back to the literal")
	}
}

func TestReplyShortcutResolvesRunFromRepliedMessage(t *testing.T) {
	b, f := uiBot(t, []operatorapi.Run{{RunID: "r1", Status: "running", Game: "red"}})
	b.rememberRunMessage(1, 100, "r1")
	b.handleMessage(context.Background(), telegramMessage{
		MessageID: 101, From: telegramUser{ID: 1}, Chat: telegramChat{ID: 1}, Text: "flag",
		ReplyToMessage: &telegramMessage{MessageID: 100},
	})
	last := f.calls[len(f.calls)-1]
	if !strings.Contains(last["text"].(string), "Flag run") {
		t.Fatalf("reply 'flag' should open the flag confirmation: %+v", last)
	}
}

func TestFlagConfirmationAcceptsNoteByReply(t *testing.T) {
	b, f := uiBot(t, []operatorapi.Run{{RunID: "r1", Status: "running", Game: "red"}})
	text, kb, err := b.askConfirmation(context.Background(), 1, 1, "flag", "r1")
	if err != nil || !strings.Contains(text, "reply") || kb == nil {
		t.Fatalf("confirmation: %q %v", text, err)
	}
	b.rememberConfirmMessage(1, 300, kb)
	b.handleMessage(context.Background(), telegramMessage{
		MessageID: 301, From: telegramUser{ID: 1}, Chat: telegramChat{ID: 1}, Text: "stuck in the PC menu",
		ReplyToMessage: &telegramMessage{MessageID: 300},
	})
	last := f.calls[len(f.calls)-1]
	if !strings.Contains(last["text"].(string), "Flagged r1") {
		t.Fatalf("note reply should flag: %+v", last)
	}
}

func TestNavigationEditsInPlace(t *testing.T) {
	b, f := uiBot(t, []operatorapi.Run{{RunID: "r1", Status: "running"}})
	b.handleCallback(context.Background(), callbackQuery{ID: "q", From: telegramUser{ID: 1}, Data: "nav:runs",
		Message: telegramMessage{MessageID: 55, Chat: telegramChat{ID: 1}}})
	var edited bool
	for _, c := range f.calls {
		if c["_method"] == "editMessageText" && c["message_id"] == float64(55) {
			edited = true
		}
	}
	if !edited {
		t.Fatalf("nav must edit the tapped message: %+v", f.calls)
	}
}

func TestBadgeBar(t *testing.T) {
	if got := badgeBar(5, 8); got != "🏅 ●●●●●○○○ 5/8" {
		t.Fatalf("%q", got)
	}
}
```

The test uses a `newBot(cfg, tg, op) *bot` constructor; extract it from `main()` in this task (all map fields initialized there) so tests and `main` share it. `rememberConfirmMessage(chat, msgID, kb)` records which confirmation token a sent confirmation message carries (read the token from the keyboard's `confirm:<token>` button) so a reply to that message can complete it.

- [ ] **Step 2: Run** `go test ./cmd/poketelegram/ -run 'RunsCard|ReplyShortcut|FlagConfirmation|NavigationEdits|BadgeBar' -count=1` → FAIL to compile.

- [ ] **Step 3: Implement** `cmd/poketelegram/ui.go`:

```go
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

const pageSize = 8

type card struct {
	Text     string
	Keyboard *inlineKeyboard
	RunID    string
}

type runSample struct {
	At    time.Time
	Frame uint64
	Maps  int
}

func btn(text, data string) inlineButton { return inlineButton{Text: text, CallbackData: data} }
func link(text, url string) inlineButton { return inlineButton{Text: text, URL: url} }

func backRow(refresh string) []inlineButton {
	return []inlineButton{btn("↻ Refresh", refresh), btn("« Menu", "nav:home")}
}

// handle maps an arbitrary value (run ID, check name) to a short stable
// token for callback data. Bounded: the map resets past 5000 entries; a stale
// button then answers "expired, refresh".
func (b *bot) handle(value string) string {
	sum := sha1.Sum([]byte(value))
	h := hex.EncodeToString(sum[:5])
	b.mu.Lock()
	if len(b.handles) > 5000 {
		b.handles = map[string]string{}
	}
	b.handles[h] = value
	b.mu.Unlock()
	return h
}

func (b *bot) unhandle(h string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.handles[h]
}

func (b *bot) resolveRun(chat int64, arg string) string {
	if n, err := strconv.Atoi(strings.TrimSpace(arg)); err == nil && n >= 1 {
		b.mu.Lock()
		list := b.lastList[chat]
		b.mu.Unlock()
		if n <= len(list) {
			return list[n-1]
		}
	}
	return strings.TrimSpace(arg)
}

func (b *bot) rememberRunMessage(chat, msgID int64, runID string) {
	if runID == "" || msgID == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.msgRuns[chat]
	if m == nil || len(m) > 500 {
		m = map[int64]string{}
		b.msgRuns[chat] = m
	}
	m[msgID] = runID
}

func (b *bot) runForMessage(chat, msgID int64) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.msgRuns[chat][msgID]
}

func badgeBar(n, total int) string {
	if total <= 0 {
		total = 8
	}
	if n > total {
		total = n
	}
	return fmt.Sprintf("🏅 %s%s %d/%d", strings.Repeat("●", n), strings.Repeat("○", total-n), n, total)
}

// placeName is game-agnostic: the runtime's generic map_name when the game
// adapter publishes one, otherwise the numeric map id.
func placeName(run operatorapi.Run) string {
	if name, ok := run.GameState["map_name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return fmt.Sprintf("map %d", run.Map)
}

func badges(run operatorapi.Run) int {
	n := run.RecoveryBadges
	if run.Player != nil && len(run.Player.Badges) > n {
		n = len(run.Player.Badges)
	}
	return n
}

func statusDot(status string) string {
	switch status {
	case "running", "leased":
		return "🟢"
	case "queued":
		return "⏳"
	}
	return "⚪"
}

func (b *bot) homeCard() card {
	return card{Text: "<b>PokePilot</b>\nPick a view.", Keyboard: &inlineKeyboard{InlineKeyboard: [][]inlineButton{
		{btn("📊 Status", "nav:status"), btn("🎮 Runs", "nav:runs")},
		{btn("🧯 Failures", "nav:failures"), btn("🩺 Health", "nav:health")},
		{btn("🔧 Fixer", "nav:fixer"), btn("🚨 Alerts", "nav:alerts")},
	}}}
}

func (b *bot) runsCard(ctx context.Context, chat int64, page int) (card, error) {
	dash, err := b.op.Dashboard(ctx, true, 50)
	if err != nil {
		return card{}, err
	}
	runs := dash.Runs
	ids := make([]string, len(runs))
	for i, r := range runs {
		ids[i] = r.RunID
	}
	b.mu.Lock()
	b.lastList[chat] = ids
	b.mu.Unlock()
	if len(runs) == 0 {
		return card{Text: "<b>Runs</b>\nNo active runs.", Keyboard: &inlineKeyboard{InlineKeyboard: [][]inlineButton{backRow("nav:runs")}}}, nil
	}
	pages := (len(runs) + pageSize - 1) / pageSize
	page = max(0, min(page, pages-1))
	lines := []string{fmt.Sprintf("<b>Runs</b> (%d)", len(runs))}
	var rows [][]inlineButton
	for i := page * pageSize; i < min(len(runs), (page+1)*pageSize); i++ {
		r := runs[i]
		label := fmt.Sprintf("%s %s · 🏅%d · %s", statusDot(r.Status), emptyDash(r.Game), badges(r), placeName(r))
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, h(label)))
		rows = append(rows, []inlineButton{btn(clip(fmt.Sprintf("%d. %s", i+1, label), 60), "run:"+b.handle(r.RunID))})
	}
	if pages > 1 {
		var nav []inlineButton
		if page > 0 {
			nav = append(nav, btn("‹", fmt.Sprintf("pg:runs:%d", page-1)))
		}
		nav = append(nav, btn(fmt.Sprintf("%d/%d", page+1, pages), fmt.Sprintf("pg:runs:%d", page)))
		if page < pages-1 {
			nav = append(nav, btn("›", fmt.Sprintf("pg:runs:%d", page+1)))
		}
		rows = append(rows, nav)
	}
	rows = append(rows, backRow(fmt.Sprintf("pg:runs:%d", page)))
	return card{Text: strings.Join(lines, "\n"), Keyboard: &inlineKeyboard{InlineKeyboard: rows}}, nil
}

func (b *bot) runCard(ctx context.Context, runID string) (card, error) {
	inspection, err := b.op.Run(ctx, runID)
	if err != nil {
		return card{}, err
	}
	run := inspection.Run
	lines := []string{
		fmt.Sprintf("%s <b>%s</b> · <code>%s</code>", statusDot(run.Status), h(emptyDash(run.Game)), h(run.RunID)),
		badgeBar(badges(run), 8),
		fmt.Sprintf("📍 %s · %d maps visited%s", h(placeName(run)), run.MapsVisited, b.lastNewMapSuffix(run.RunID)),
		fmt.Sprintf("🎞 frame %d%s", run.Frame, b.rateSuffix(run.RunID)),
		fmt.Sprintf("🔁 attempt %d · lost %d · recoveries %d", run.Attempts, run.LossRecoveries, run.RecoveryEvents),
		"🧠 " + plannerState(run),
		"🎯 " + h(clip(emptyDash(runGoal(run)), 200)),
	}
	if run.Reason != "" {
		lines = append(lines, "Result: "+h(run.Reason+detailSuffix(run.Detail)))
	}
	if group, gErr := b.op.FindTriageForRun(ctx, runID); gErr == nil && group != nil {
		issue := ""
		if group.Issue != nil && group.Issue.IssueNumber > 0 {
			issue = fmt.Sprintf(" · issue #%d", group.Issue.IssueNumber)
		}
		lines = append(lines, fmt.Sprintf("🧯 x%d %s%s", group.Count, h(clip(firstNonEmpty(group.Pattern, group.Example), 160)), issue))
	}
	hd := b.handle(runID)
	kb := &inlineKeyboard{InlineKeyboard: [][]inlineButton{
		{btn("🖼 Frame", "act:frame:"+hd), btn("🎬 Replay", "act:replay:"+hd), btn("🧯 Triage", "act:triage:"+hd)},
		{btn("🚩 Flag stuck", "act:flag:"+hd), btn("⏹ Stop", "act:stop:"+hd), btn("🔄 Restart", "act:restart:"+hd)},
		{link("Admin", b.adminRunURL(runID)), link("Spectator", b.spectatorRunURL(runID))},
		{btn("↻ Refresh", "run:"+hd), btn("« Runs", "nav:runs")},
	}}
	return card{Text: strings.Join(lines, "\n"), Keyboard: kb, RunID: runID}, nil
}

func plannerState(run operatorapi.Run) string {
	switch {
	case !isActive(run.Status):
		return emptyDash(run.Status)
	case run.Question != "" && run.Decision == "":
		return "thinking"
	case run.Decision != "":
		return "acting: " + h(clip(run.Decision, 120))
	}
	return "idle"
}

// recordSample keeps 15 minutes of (frame, maps) per active run from the
// monitor tick, for frames/min and "last new map".
func (b *bot) recordSample(run operatorapi.Run, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := append(b.samples[run.RunID], runSample{At: now, Frame: run.Frame, Maps: run.MapsVisited})
	for len(s) > 0 && now.Sub(s[0].At) > 15*time.Minute {
		s = s[1:]
	}
	b.samples[run.RunID] = s
	if len(s) >= 2 && s[len(s)-1].Maps > s[len(s)-2].Maps {
		b.lastNewMap[run.RunID] = now
	}
}

func (b *bot) rateSuffix(runID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.samples[runID]
	if len(s) < 2 {
		return ""
	}
	span := s[len(s)-1].At.Sub(s[0].At).Minutes()
	if span <= 0 {
		return ""
	}
	return fmt.Sprintf(" · %.0f/min", float64(s[len(s)-1].Frame-s[0].Frame)/span)
}

func (b *bot) lastNewMapSuffix(runID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if at, ok := b.lastNewMap[runID]; ok {
		return " · last new map " + durationShort(time.Since(at)) + " ago"
	}
	return ""
}
```

Also in `ui.go`: `failuresCard` (port `failuresText` to HTML with paging, `pg:fail:<n>`), `alertsCard` (open alerts from `b.book.Open()` under `b.alertMu`, with mute buttons, plus Alertmanager lines from `alertsText`), `healthCard`, `fixerCard` (render `b.opsSnapshot()`; nil → "No watcher data yet."):

```go
func (b *bot) healthCard() card {
	s := b.opsSnapshot()
	rows := [][]inlineButton{backRow("nav:health")}
	if s == nil {
		return card{Text: "<b>Health</b>\nNo watcher data yet.", Keyboard: &inlineKeyboard{InlineKeyboard: rows}}
	}
	lines := []string{fmt.Sprintf("<b>Health</b> · %s ago", durationShort(time.Since(time.Unix(s.At, 0))))}
	managers, up := 0, 0
	var nodeLines []string
	for _, n := range s.Nodes {
		icon := "🟢"
		if n.Status != "ready" || n.ManagerStatus == "unreachable" {
			icon = "🔴"
		}
		if n.Manager {
			managers++
			if icon == "🟢" {
				up++
			}
		}
		role := ""
		if n.Manager {
			role = " (" + n.ManagerStatus + ")"
		}
		nodeLines = append(nodeLines, fmt.Sprintf("%s %s%s", icon, h(n.Hostname), h(role)))
	}
	lines = append(lines, fmt.Sprintf("<b>Nodes</b> · managers %d/%d", up, managers))
	lines = append(lines, nodeLines...)
	lines = append(lines, "<b>Services</b>")
	for _, svc := range s.Services {
		icon := "🟢"
		if svc.Running < svc.Desired || strings.HasPrefix(svc.UpdateState, "rollback") {
			icon = "🔴"
		}
		lines = append(lines, fmt.Sprintf("%s %s %d/%d%s", icon, h(svc.Name), svc.Running, svc.Desired, h(suffixIf(svc.UpdateState))))
	}
	if s.FrozenUntil > time.Now().Unix() {
		lines = append(lines, "🧊 deploys frozen until "+time.Unix(s.FrozenUntil, 0).UTC().Format("Jan 2 15:04 UTC"))
	}
	type disk struct {
		node, mount string
		free        int
	}
	var disks []disk
	for _, r := range s.Disks {
		for _, d := range r.Disks {
			disks = append(disks, disk{r.Node, d.Mount, d.FreeGB})
		}
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].free < disks[j].free })
	lines = append(lines, "<b>Disk free</b> (lowest first)")
	for i, d := range disks {
		if i == 6 {
			break
		}
		lines = append(lines, fmt.Sprintf("%dG %s %s", d.free, h(d.node), h(d.mount)))
	}
	return card{Text: strings.Join(lines, "\n"), Keyboard: &inlineKeyboard{InlineKeyboard: rows}}
}

func suffixIf(s string) string {
	if s == "" || s == "completed" {
		return ""
	}
	return " · " + s
}

func (b *bot) fixerCard() card {
	s := b.opsSnapshot()
	rows := [][]inlineButton{backRow("nav:fixer")}
	if s == nil || s.Fixer == nil {
		return card{Text: "<b>Fixer</b>\nNo fixer report yet.", Keyboard: &inlineKeyboard{InlineKeyboard: rows}}
	}
	f := s.Fixer
	rate := "—"
	if f.Attempts24h > 0 {
		rate = fmt.Sprintf("%d%%", 100*f.PRs24h/f.Attempts24h)
	}
	lines := []string{
		"<b>Fixer</b> (24h)",
		fmt.Sprintf("💸 paid %d/%d · 🆓 qwen %d", f.PaidStarts24h, f.PaidCap, f.FreeStarts24h),
		fmt.Sprintf("🛠 %d attempts → %d PRs (%s)", f.Attempts24h, f.PRs24h, rate),
		fmt.Sprintf("✅ merged PRs: %s", countText(s.Merged24h)),
	}
	if len(f.BlockedKeys) > 0 {
		lines = append(lines, "<b>Blocked keys</b>")
		for i, k := range f.BlockedKeys {
			if i == 8 {
				lines = append(lines, fmt.Sprintf("…%d more", len(f.BlockedKeys)-8))
				break
			}
			lines = append(lines, "• <code>"+h(k)+"</code>")
		}
	}
	if len(s.TriagePRs) > 0 {
		lines = append(lines, "<b>Open fixer PRs</b>")
		for i, pr := range s.TriagePRs {
			if i == 8 {
				break
			}
			lines = append(lines, fmt.Sprintf(`• <a href="%s">#%d</a> %s old`, h(pr.URL), pr.Number, durationShort(time.Since(time.Unix(pr.CreatedAt, 0)))))
		}
	}
	return card{Text: strings.Join(lines, "\n"), Keyboard: &inlineKeyboard{InlineKeyboard: rows}}
}

func countText(n int) string {
	if n < 0 {
		return "?"
	}
	return strconv.Itoa(n)
}
```

Blocked keys link to their issue on the Fixer card: when `b.op.Triage(ctx)` succeeds, map `group.Key → group.Issue.IssueNumber` and render `<a href="https://github.com/<repo>/issues/N">` (repo from `POKEPILOT_GITHUB_REPO`, default `maestroi/PokePilot`, added to `config`). Make `fixerCard` take `ctx` for that lookup; if Triage fails, keys render without links.

`main.go` changes:

1. Extract `newBot(cfg config, tg *telegramClient, op *operatorapi.Client) *bot` from `main` initializing all maps (existing + `handles`, `lastList`, `msgRuns`, `samples`, `lastNewMap`, `confirmMsgs map[int64]map[int64]string`), and `book: newAlertBook(12*time.Hour)` (field used by Task 9 and `alertsCard`), and add `alertMu sync.Mutex` to `bot`.
2. In `main`, after building the bot: `_ = b.tg.setCommands(ctx, botCommands)` with

```go
var botCommands = [][2]string{
	{"menu", "Open the menu"}, {"runs", "Active runs"}, {"health", "Swarm, disk and deploy health"},
	{"fixer", "Fixer budget, blocked keys and PRs"}, {"failures", "Failure groups"}, {"alerts", "Open alerts"},
	{"board", "Post a live board to pin"}, {"status", "One-line farm summary"}, {"help", "All commands"},
}
```

3. `handleMessage`:
   - Before `parseCommand`, if `msg.ReplyToMessage != nil`:
     - if the replied message is a pending confirmation (`b.confirmMsgs[chat][replyID]` → token) and that confirmation's action is `flag`, run the confirm path with `note = msg.Text`.
     - else if `runForMessage(chat, replyID)` returns a run and the lowercased text is one of `flag|stop|restart|frame|run|replay|triage`, rewrite to that command with the run as arg.
   - Commands: `start`/`menu` → send `homeCard()`; `runs` → `runsCard(page 0)`; `run`, `flag`, `stop`, `restart`, `triage`, `replay` → `arg = b.resolveRun(chat, arg)` first; `run` sends `runCard`; `flag` → `askConfirmation(..., "flag", arg)`; `health`/`fixer`/`failures`/`alerts` → the cards; `board` (Task 9).
   - Every sent card goes through one helper that records run messages:

```go
func (b *bot) sendCard(ctx context.Context, chat int64, c card) {
	id, err := b.tg.send(ctx, chat, outgoing{Text: c.Text, HTML: true, Keyboard: c.Keyboard})
	if err != nil {
		b.m.telegramErrs.Add(1)
		log.Printf("poketelegram: send card: %v", err)
		return
	}
	b.rememberRunMessage(chat, id, c.RunID)
}

func (b *bot) editCard(ctx context.Context, chat, msgID int64, c card) {
	err := b.tg.edit(ctx, chat, msgID, outgoing{Text: c.Text, HTML: true, Keyboard: c.Keyboard})
	switch {
	case err == nil, errors.Is(err, errNotModified):
		b.rememberRunMessage(chat, msgID, c.RunID)
	case errors.Is(err, errMessageGone):
		b.sendCard(ctx, chat, c)
	default:
		b.m.telegramErrs.Add(1)
		log.Printf("poketelegram: edit card: %v", err)
	}
}
```

4. `handleCallback`: keep `confirm:`/`cancel:` handling; add routing for the grammar above, editing `cb.Message.MessageID` via `editCard`. `run:<h>` with unknown handle → edit to "That button expired. Tap ↻ Refresh." `act:frame` → `sendRunFrame`; `act:replay`/`act:triage` → existing `queueReplay`/`runTriage` and send result text; `act:flag|stop|restart` → `askConfirmation` sent as a new message, then `rememberConfirmMessage`. Confirm path gains:

```go
	case "flag":
		err = b.op.FlagStuck(ctx, pending.Target, pending.Note)
		if err == nil {
			text = "🚩 Flagged " + pending.Target + " as stuck. I'll post the issue link when triage files it."
			b.watchFlag(pending.Target, cb.Message.Chat.ID) // Task 9
		}
```

   Add `Note string` to `confirmation`. Refactor the body of the confirm branch into `func (b *bot) confirm(ctx context.Context, actor, chat int64, token, note string) string` so both the button and the reply path call it.

5. `askConfirmation` for `flag` returns text `"🚩 Flag run <id> as stuck?\n<summary>\nTo add a note, reply to this message with it. Otherwise tap Confirm."` (plain text is fine here; keep existing format for stop/restart).
6. `observeRuns`: call `b.recordSample(run, now)` for active runs.
7. `helpText`: list menu, runs, run N, flag N, health, fixer, board, and the reply shortcuts.

- [ ] **Step 4: Run** `go test ./cmd/poketelegram/ -count=1` → PASS (new + existing). Fix any existing test that relied on the old command texts only if the behavior it pinned was intentionally changed (it should not be: typed `/status`, `/run <id>`, `/failures` keep their old text replies).

- [ ] **Step 5: Commit**

```bash
git add cmd/poketelegram/
git commit -m "feat(poketelegram): tap-driven cards, numbered runs, reply shortcuts and flag-stuck flow"
```

---

### Task 9: Ops ingest, alert rendering, board, digest, flag follow-up

**Files:**
- Create: `cmd/poketelegram/ops.go`, `cmd/poketelegram/ops_test.go`
- Modify: `cmd/poketelegram/main.go` (`httpHandler` route, `monitorOnce`, `observeRuns` stall branch, `loadConfig`)

**Interfaces:**
- Consumes: Tasks 2, 6, 7, 8.
- Produces: `POST /v1/ops` (bearer `POKEPILOT_OPS_TOKEN_FILE`) storing `operatorapi.OpsSnapshot`; `func (b *bot) opsSnapshot() *operatorapi.OpsSnapshot`; `func (b *bot) localChecks(runs []operatorapi.Run, wallErr error, now time.Time) []operatorapi.CheckResult`; `func (b *bot) applyAlerts(ctx context.Context, source string, results []operatorapi.CheckResult, now time.Time)`; `func (b *bot) watchFlag(runID string, chat int64)`; `func (b *bot) digestText(now time.Time) (string, *inlineKeyboard)`; board helpers.
- Config additions: `OpsToken string` (from `POKEPILOT_OPS_TOKEN_FILE`), `DigestHour int` (`POKEPILOT_WATCH_DIGEST_HOUR`, default 9), `GitHubRepo string`.

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func TestOpsEndpointRequiresTokenAndStoresSnapshot(t *testing.T) {
	b, _ := uiBot(t, nil)
	b.cfg.OpsToken = "tok"
	body, _ := json.Marshal(operatorapi.OpsSnapshot{At: 5})
	rec := httptest.NewRecorder()
	b.httpHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/ops", bytes.NewReader(body)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/ops", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	b.httpHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || b.opsSnapshot() == nil || b.opsSnapshot().At != 5 {
		t.Fatalf("stored: %d %+v", rec.Code, b.opsSnapshot())
	}
}

func TestLocalChecksWallStallAndWatcherSilence(t *testing.T) {
	b, _ := uiBot(t, nil)
	now := time.Now()
	b.started = now.Add(-10 * time.Minute)
	b.runs["r1"] = runWatch{Status: "running", Frame: 10, LastProgress: now.Add(-20 * time.Minute)}
	got := map[string]operatorapi.CheckResult{}
	for _, c := range b.localChecks([]operatorapi.Run{{RunID: "r1", Status: "running", Frame: 10}}, nil, now) {
		got[c.Name] = c
	}
	if c := got["stall:r1"]; c.OK || c.RunID != "r1" {
		t.Fatalf("stall: %+v", c)
	}
	if c := got["watcher"]; c.OK {
		t.Fatalf("no push for 10m → watcher silent: %+v", c)
	}
	got = map[string]operatorapi.CheckResult{}
	for _, c := range b.localChecks(nil, errors.New("dial tcp: refused"), now) {
		got[c.Name] = c
	}
	if c := got["wall"]; c.OK || c.Grace != 2 {
		t.Fatalf("wall: %+v", c)
	}
}

func TestAlertCardOpenThenResolveEditsAndReplies(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	ctx := context.Background()
	t0 := time.Now()
	b.applyAlerts(ctx, "watch", nil, t0)
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "disk:n1:/", Grace: 1, Message: "n1 / has 5G free"}}, t0)
	open := f.calls[len(f.calls)-1]
	if open["_method"] != "sendMessage" || !strings.Contains(open["text"].(string), "n1 / has 5G free") {
		t.Fatalf("open card: %+v", open)
	}
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "disk:n1:/", OK: true}}, t0.Add(34*time.Minute))
	var edited, replied bool
	for _, c := range f.calls {
		if c["_method"] == "editMessageText" && strings.Contains(c["text"].(string), "resolved after 34m") {
			edited = true
		}
		if rp, ok := c["reply_parameters"].(map[string]any); ok && rp["message_id"] == float64(42) {
			replied = true
		}
	}
	if !edited || !replied {
		t.Fatalf("resolve must edit the card and reply to it: %+v", f.calls)
	}
}

func TestRestartSummaryIsOneMessage(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	b.applyAlerts(context.Background(), "watch", []operatorapi.CheckResult{
		{Name: "quorum", Grace: 2, Message: "2/3 managers reachable"},
		{Name: "paid-cap", Grace: 1, Message: "cap"},
	}, time.Now())
	if len(f.calls) != 1 || !strings.Contains(f.calls[0]["text"].(string), "2 checks failing") {
		t.Fatalf("one summary expected: %+v", f.calls)
	}
}

func TestStallAlertCarriesRunButtons(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	ctx := context.Background()
	b.applyAlerts(ctx, "bot", nil, time.Now())
	b.applyAlerts(ctx, "bot", []operatorapi.CheckResult{{Name: "stall:r1", Grace: 1, RunID: "r1", Message: "r1 no frame progress for 20m"}}, time.Now())
	kb, _ := json.Marshal(f.calls[len(f.calls)-1]["reply_markup"])
	if !strings.Contains(string(kb), "act:flag:") || !strings.Contains(string(kb), "mute:") {
		t.Fatalf("stall alert buttons: %s", kb)
	}
	if b.runForMessage(1, 42) != "r1" {
		t.Fatal("alert message must support reply shortcuts")
	}
}

func TestDigestIncludesBadgeDeltaAndFixer(t *testing.T) {
	b, _ := uiBot(t, nil)
	b.ops = &operatorapi.OpsSnapshot{Merged24h: 4, FarmOpened: 2, FarmClosed: 3,
		Fixer: &operatorapi.FixerSummary{PaidStarts24h: 3, PaidCap: 20, FreeStarts24h: 5, Attempts24h: 8, PRs24h: 2}}
	b.digestBadges = map[string]int{"r1": 3}
	b.lastDash = []operatorapi.Run{{RunID: "r1", Status: "running", Game: "red", RecoveryBadges: 5}}
	text, kb := b.digestText(time.Now())
	for _, want := range []string{"PRs merged: 4", "opened 2", "closed 3", "qwen 5", "paid 3/20", "25%", "(+2)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("digest missing %q:\n%s", want, text)
		}
	}
	if kb == nil {
		t.Fatal("digest buttons")
	}
}

func TestBoardDroppedWhenMessageGone(t *testing.T) {
	b, f := uiBot(t, nil)
	b.boards[1] = 77
	f.reply = func(string, map[string]any) (int, string) {
		return 400, `{"ok":false,"description":"Bad Request: message to edit not found"}`
	}
	b.refreshBoards(context.Background())
	if _, ok := b.boards[1]; ok {
		t.Fatal("a deleted board must be forgotten, not retried forever")
	}
}
```

- [ ] **Step 2: Run** `go test ./cmd/poketelegram/ -run 'OpsEndpoint|LocalChecks|AlertCard|RestartSummary|StallAlert|Digest|Board' -count=1` → FAIL to compile.

- [ ] **Step 3: Implement** `cmd/poketelegram/ops.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

const watcherSilentAfter = 5 * time.Minute

func (b *bot) opsSnapshot() *operatorapi.OpsSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ops
}

func (b *bot) handleOps(w http.ResponseWriter, r *http.Request) {
	if !operatorapi.OpsAuthorized(r, b.cfg.OpsToken) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var snap operatorapi.OpsSnapshot
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&snap); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	b.ops = &snap
	b.opsAt = time.Now()
	b.mu.Unlock()
	b.applyAlerts(r.Context(), "watch", snap.Checks, time.Now())
}

// localChecks are the checks the bot computes itself each monitor tick.
func (b *bot) localChecks(runs []operatorapi.Run, wallErr error, now time.Time) []operatorapi.CheckResult {
	var out []operatorapi.CheckResult
	if wallErr != nil {
		out = append(out, operatorapi.CheckResult{Name: "wall", Grace: 2, Message: "wall/operator API unreachable: " + clip(wallErr.Error(), 200)})
	} else {
		out = append(out, operatorapi.CheckResult{Name: "wall", OK: true, Grace: 2})
		for _, run := range runs {
			if !isActive(run.Status) {
				continue
			}
			b.mu.Lock()
			w, seen := b.runs[run.RunID]
			b.mu.Unlock()
			stalled := seen && !w.LastProgress.IsZero() && now.Sub(w.LastProgress) >= b.cfg.StallAfter
			c := operatorapi.CheckResult{Name: "stall:" + run.RunID, Grace: 1, RunID: run.RunID, OK: !stalled}
			if stalled {
				c.Message = fmt.Sprintf("%s (%s) no frame progress for %s", run.RunID, emptyDash(run.Game), durationShort(now.Sub(w.LastProgress)))
			}
			out = append(out, c)
		}
		out = append(out, b.plannerChecks(runs)...)
	}
	b.mu.Lock()
	opsAt := b.opsAt
	b.mu.Unlock()
	silent := (opsAt.IsZero() && now.Sub(b.started) >= watcherSilentAfter) || (!opsAt.IsZero() && now.Sub(opsAt) >= watcherSilentAfter)
	c := operatorapi.CheckResult{Name: "watcher", Grace: 1, OK: !silent}
	if silent {
		c.Message = "no report from pokewatch for 5m+: Swarm, disk and fixer checks are blind"
	}
	return append(out, c)
}

// plannerChecks probes each live run's planner endpoint /health, as the
// retired farm-watch.sh did. Endpoints are cached for 60s per tick cadence by
// the caller only running this on the monitor tick.
func (b *bot) plannerChecks(runs []operatorapi.Run) []operatorapi.CheckResult {
	seen := map[string]bool{}
	var out []operatorapi.CheckResult
	client := &http.Client{Timeout: 10 * time.Second}
	for _, run := range runs {
		if run.Stats == nil || !isActive(run.Status) {
			continue
		}
		ep := strings.TrimRight(run.Stats.Endpoint, "/")
		if !strings.HasPrefix(ep, "http") || seen[ep] {
			continue
		}
		seen[ep] = true
		res, err := client.Get(ep + "/health")
		healthy := err == nil && res.StatusCode < 500
		if res != nil {
			res.Body.Close()
		}
		c := operatorapi.CheckResult{Name: "planner:" + ep, Grace: 2, OK: healthy}
		if !healthy {
			c.Message = "planner endpoint unreachable: " + ep + " (runs stall until it returns)"
		}
		out = append(out, c)
	}
	return out
}

// applyAlerts is called from the monitor loop and from the /v1/ops handler,
// so b.alertMu serializes all alertBook access and alert-card bookkeeping.
func (b *bot) applyAlerts(ctx context.Context, source string, results []operatorapi.CheckResult, now time.Time) {
	b.alertMu.Lock()
	defer b.alertMu.Unlock()
	actions, adopted := b.book.Observe(source, results, now)
	if len(adopted) > 0 {
		lines := []string{fmt.Sprintf("🔁 <b>Bot restarted</b>: %d checks failing", len(adopted))}
		for _, st := range adopted {
			lines = append(lines, "🔴 "+h(clip(firstNonEmpty(st.Message, st.Name), 200)))
		}
		b.notifyHTML(ctx, outgoing{Text: strings.Join(lines, "\n"), HTML: true, Keyboard: &inlineKeyboard{InlineKeyboard: [][]inlineButton{{btn("🚨 Alerts", "nav:alerts"), btn("🩺 Health", "nav:health")}}}})
	}
	for _, a := range actions {
		switch a.Kind {
		case alertOpen:
			b.sendAlertCard(ctx, a.State, now)
		case alertRemind:
			for chat, msg := range a.State.Cards {
				b.notifyOne(ctx, chat, outgoing{Text: "🔴 still failing (" + durationShort(now.Sub(a.State.Since)) + "): " + h(clip(a.State.Message, 300)), HTML: true, ReplyTo: msg})
			}
			if len(a.State.Cards) == 0 {
				b.sendAlertCard(ctx, a.State, now)
			}
		case alertResolve:
			for chat, msg := range a.State.Cards {
				text := fmt.Sprintf("✅ <b>%s</b> resolved after %s\n<s>%s</s>", h(a.State.Name), durationShort(now.Sub(a.State.Since)), h(clip(a.State.Message, 300)))
				if err := b.tg.edit(ctx, chat, msg, outgoing{Text: text, HTML: true}); err != nil && !errors.Is(err, errNotModified) {
					b.m.telegramErrs.Add(1)
				}
				b.notifyOne(ctx, chat, outgoing{Text: "✅ resolved", ReplyTo: msg, Silent: true})
			}
		}
	}
}

func (b *bot) sendAlertCard(ctx context.Context, st *alertState, now time.Time) {
	hd := b.handle(st.Name)
	rows := [][]inlineButton{}
	if st.RunID != "" {
		rh := b.handle(st.RunID)
		rows = append(rows, []inlineButton{btn("🚩 Flag stuck", "act:flag:"+rh), btn("🎮 Open run", "run:"+rh), btn("🖼 Frame", "act:frame:"+rh)})
	}
	row := []inlineButton{btn("🔕 Mute 12h", "mute:"+hd), btn("ℹ️ Details", "nav:alerts")}
	if st.Link != "" {
		row = append(row, link("Open", st.Link))
	}
	rows = append(rows, row)
	text := fmt.Sprintf("🔴 <b>%s</b>\n%s\nfailing for %s", h(st.Name), h(clip(st.Message, 500)), durationShort(now.Sub(st.Since)))
	for _, chat := range b.cfg.NotifyChats {
		id, err := b.tg.send(ctx, chat, outgoing{Text: text, HTML: true, Keyboard: &inlineKeyboard{InlineKeyboard: rows}})
		if err != nil {
			b.m.telegramErrs.Add(1)
			log.Printf("poketelegram: alert %s to %d: %v", st.Name, chat, err)
			continue
		}
		b.m.notifications.Add(1)
		st.Cards[chat] = id // caller holds b.alertMu
		b.rememberRunMessage(chat, id, st.RunID)
	}
}

func (b *bot) notifyOne(ctx context.Context, chat int64, m outgoing) {
	if _, err := b.tg.send(ctx, chat, m); err != nil {
		b.m.telegramErrs.Add(1)
		return
	}
	b.m.notifications.Add(1)
}

func (b *bot) notifyHTML(ctx context.Context, m outgoing) {
	for _, chat := range b.cfg.NotifyChats {
		b.notifyOne(ctx, chat, m)
	}
}
```

Flag follow-up (same file):

```go
type flagWatch struct {
	Chat  int64
	Since time.Time
}

func (b *bot) watchFlag(runID string, chat int64) {
	b.mu.Lock()
	b.flags[runID] = flagWatch{Chat: chat, Since: time.Now()}
	b.mu.Unlock()
}

// observeFlags posts the issue link once triage files the flagged failure,
// or gives up after an hour with a link to the run.
func (b *bot) observeFlags(ctx context.Context, now time.Time) {
	b.mu.Lock()
	pending := make(map[string]flagWatch, len(b.flags))
	for k, v := range b.flags {
		pending[k] = v
	}
	b.mu.Unlock()
	for runID, f := range pending {
		group, err := b.op.FindTriageForRun(ctx, runID)
		done := false
		if err == nil && group != nil && group.Issue != nil && group.Issue.IssueNumber > 0 {
			b.notifyOne(ctx, f.Chat, outgoing{HTML: true, Text: fmt.Sprintf(`🧯 Flagged run <code>%s</code> is filed as <a href="https://github.com/%s/issues/%d">issue #%d</a> (triage key <code>%s</code>).`,
				h(runID), h(b.cfg.GitHubRepo), group.Issue.IssueNumber, group.Issue.IssueNumber, h(group.Key))})
			done = true
		} else if now.Sub(f.Since) > time.Hour {
			b.notifyOne(ctx, f.Chat, outgoing{Text: "No issue was filed for flagged run " + runID + " within an hour. If the runner never stopped, the wall settled it without a dump; check " + b.adminRunURL(runID)})
			done = true
		}
		if done {
			b.mu.Lock()
			delete(b.flags, runID)
			b.mu.Unlock()
		}
	}
}
```

Board and digest (same file):

```go
func (b *bot) boardText() string {
	lines := []string{"📌 <b>PokePilot live</b> · " + time.Now().UTC().Format("15:04 UTC")}
	b.mu.Lock()
	runs := append([]operatorapi.Run(nil), b.lastDash...)
	snap := b.ops
	b.mu.Unlock()
	b.alertMu.Lock()
	open := b.book.Open()
	b.alertMu.Unlock()
	active := 0
	for _, r := range runs {
		if !isActive(r.Status) {
			continue
		}
		active++
		if active <= 8 {
			lines = append(lines, fmt.Sprintf("%s %s 🏅%d %s%s", statusDot(r.Status), h(emptyDash(r.Game)), badges(r), h(placeName(r)), b.rateSuffix(r.RunID)))
		}
	}
	lines = append(lines, fmt.Sprintf("Runs: %d active", active))
	if len(open) == 0 {
		lines = append(lines, "✅ no open alerts")
	} else {
		lines = append(lines, fmt.Sprintf("🔴 %d open alerts", len(open)))
		for i, st := range open {
			if i == 5 {
				break
			}
			lines = append(lines, "• "+h(clip(firstNonEmpty(st.Message, st.Name), 120)))
		}
	}
	if snap != nil && snap.Fixer != nil {
		lines = append(lines, fmt.Sprintf("🔧 paid %d/%d · qwen %d · PRs %d", snap.Fixer.PaidStarts24h, snap.Fixer.PaidCap, snap.Fixer.FreeStarts24h, snap.Fixer.PRs24h))
	}
	if snap != nil && snap.FrozenUntil > time.Now().Unix() {
		lines = append(lines, "🧊 deploys frozen")
	}
	return strings.Join(lines, "\n")
}

var boardKeyboard = &inlineKeyboard{InlineKeyboard: [][]inlineButton{{btn("🎮 Runs", "nav:runs"), btn("🩺 Health", "nav:health"), btn("🔧 Fixer", "nav:fixer")}}}

func (b *bot) startBoard(ctx context.Context, chat int64) {
	id, err := b.tg.send(ctx, chat, outgoing{Text: b.boardText(), HTML: true, Keyboard: boardKeyboard, Silent: true})
	if err != nil {
		b.m.telegramErrs.Add(1)
		return
	}
	b.mu.Lock()
	b.boards[chat] = id
	b.mu.Unlock()
	b.notifyOne(ctx, chat, outgoing{Text: "Pin the message above to keep it handy; it refreshes every minute without notifying.", ReplyTo: id, Silent: true})
}

func (b *bot) refreshBoards(ctx context.Context) {
	b.mu.Lock()
	boards := make(map[int64]int64, len(b.boards))
	for k, v := range b.boards {
		boards[k] = v
	}
	b.mu.Unlock()
	text := b.boardText()
	for chat, id := range boards {
		err := b.tg.edit(ctx, chat, id, outgoing{Text: text, HTML: true, Keyboard: boardKeyboard})
		if errors.Is(err, errMessageGone) {
			b.mu.Lock()
			delete(b.boards, chat)
			b.mu.Unlock()
		} else if err != nil && !errors.Is(err, errNotModified) {
			b.m.telegramErrs.Add(1)
		}
	}
}

func (b *bot) digestText(now time.Time) (string, *inlineKeyboard) {
	b.mu.Lock()
	snap := b.ops
	runs := append([]operatorapi.Run(nil), b.lastDash...)
	prev := b.digestBadges
	b.mu.Unlock()
	lines := []string{"📊 <b>PokePilot daily</b>"}
	if snap != nil {
		lines = append(lines, fmt.Sprintf("PRs merged: %s · farm issues opened %s, closed %s", countText(snap.Merged24h), countText(snap.FarmOpened), countText(snap.FarmClosed)))
		if f := snap.Fixer; f != nil {
			rate := "—"
			if f.Attempts24h > 0 {
				rate = fmt.Sprintf("%d%%", 100*f.PRs24h/f.Attempts24h)
			}
			lines = append(lines, fmt.Sprintf("Fixer: qwen %d, paid %d/%d · %d attempts → %d PRs (%s)", f.FreeStarts24h, f.PaidStarts24h, f.PaidCap, f.Attempts24h, f.PRs24h, rate))
		}
		if snap.FrozenUntil > now.Unix() {
			lines = append(lines, "🧊 deploys frozen")
		}
	} else {
		lines = append(lines, "No watcher data.")
	}
	cur := map[string]int{}
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID < runs[j].RunID })
	for _, r := range runs {
		if r.Status == "done" {
			continue
		}
		n := badges(r)
		cur[r.RunID] = n
		delta := ""
		if p, ok := prev[r.RunID]; ok {
			if n == p {
				delta = " (no change)"
			} else {
				delta = fmt.Sprintf(" (%+d)", n-p)
			}
		}
		lines = append(lines, fmt.Sprintf("• %s %s: 🏅%d%s · attempt %d, lost %d", h(emptyDash(r.Game)), h(r.PlayStyle), n, delta, r.Attempts, r.LossRecoveries))
	}
	var muted []string
	b.alertMu.Lock()
	for _, st := range b.book.Open() {
		if st.MutedUntil.After(now) {
			muted = append(muted, st.Name)
		}
	}
	b.alertMu.Unlock()
	b.mu.Lock()
	b.digestBadges = cur
	b.mu.Unlock()
	if len(muted) > 0 {
		lines = append(lines, "🔕 muted: "+h(strings.Join(muted, ", ")))
	}
	return strings.Join(lines, "\n"), boardKeyboard
}

func (b *bot) maybeDigest(ctx context.Context, now time.Time) {
	day := now.Format("2006-01-02")
	if now.Hour() < b.cfg.DigestHour || b.digestDay == day {
		return
	}
	b.digestDay = day
	text, kb := b.digestText(now)
	b.notifyHTML(ctx, outgoing{Text: text, HTML: true, Keyboard: kb})
}
```

The digest test calls `digestText` directly and expects `"(+2)"`, `"25%"` (2 PRs / 8 attempts), `"qwen 5"`, `"paid 3/20"`, `"opened 2"`, `"closed 3"`, `"PRs merged: 4"`: the format strings above produce exactly those.

`main.go` wiring:

- `bot` gains: `alertMu sync.Mutex` (guards `book` and every `alertState`; never held while taking `b.mu` for longer than a map access), `book *alertBook`, `ops *operatorapi.OpsSnapshot`, `opsAt time.Time`, `started time.Time`, `lastDash []operatorapi.Run`, `flags map[string]flagWatch`, `boards map[int64]int64`, `digestBadges map[string]int`, `digestDay string`. Initialize in `newBot` (`started: time.Now()`).
- `httpHandler`: `mux.HandleFunc("POST /v1/ops", b.handleOps)`.
- `loadConfig`: `OpsToken: operatorapi.ReadSecretFile(os.Getenv("POKEPILOT_OPS_TOKEN_FILE"))`, `DigestHour` via `strconv.Atoi(envDefault("POKEPILOT_WATCH_DIGEST_HOUR","9"))`, `GitHubRepo: envDefault("POKEPILOT_GITHUB_REPO", "maestroi/PokePilot")`.
- `monitorOnce` becomes:

```go
func (b *bot) monitorOnce(ctx context.Context) {
	now := time.Now()
	dash, err := b.op.Dashboard(ctx, false, 200)
	if err != nil {
		b.m.operatorErrs.Add(1)
		b.m.wallHealthy.Store(0)
	} else {
		b.m.wallHealthy.Store(1)
		b.m.lastPollUnix.Store(now.Unix())
		b.mu.Lock()
		b.lastDash = dash.Runs
		b.mu.Unlock()
		b.observeRuns(ctx, dash.Runs)
		b.observeRenderJobs(ctx)
		b.observeFlags(ctx, now)
	}
	b.applyAlerts(ctx, "bot", b.localChecks(dash.Runs, err, now), now)
	b.observeAlerts(ctx)
	if now.Sub(b.boardAt) >= time.Minute {
		b.boardAt = now
		b.refreshBoards(ctx)
	}
	b.maybeDigest(ctx, now)
}
```

  (`dash` is the zero value on error, so `dash.Runs` is nil there; add `boardAt time.Time` to `bot`.)
- Remove the `wallDown` notification branches and the `StallNotified` notify branch in `observeRuns` (keep `LastProgress` tracking — `localChecks` reads it). Remove `wallDown`, `StallNotified` fields.
- `handleMessage`: `board` → `b.startBoard(ctx, msg.Chat.ID)`.
- `handleCallback`: `mute:<h>` → `b.book.Mute(b.unhandle(h), now+12h)` under `b.alertMu`, answer by editing the alert card to append "🔕 muted until HH:MM UTC".
- Update the existing test that asserted on the old wall-down/stall notification texts, if any, to the new alert-card behavior.

- [ ] **Step 4: Run** `go test ./cmd/poketelegram/ -count=1 -race` → PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/poketelegram/
git commit -m "feat(poketelegram): ops ingest, alert cards, live board, daily digest and flag follow-up"
```

---

### Task 10: Deploy, docs, and removal of farm-watch

**Files:**
- Create: `deploy/ops.yml`, `deploy/ops_config_test.go`
- Delete: `deploy/telegram.yml`, `deploy/farm-watch.sh`, `deploy/farm-watch.service.in`, `deploy/farm-watch.timer`, `deploy/farm_watch_test.go`, `deploy/qwagent-triage.service.in`, `deploy/qwagent-triage.timer`, `deploy/qwagent-triage.zsh`
- Modify: `deploy/Dockerfile` (build `pokewatch`), `Makefile` (drop `qwagent-triage-install`), `deploy/README.md`, `docs/TELEGRAM_OPERATOR.md`, `deploy/monitoring/README.md` (rename stack references)

**Interfaces:**
- Consumes: env/flags from Tasks 3, 4, 9.

- [ ] **Step 1: Check what still uses the files being deleted**

Run: `grep -rn "farm-watch\|farm_watch\|qwagent-triage\.\(service\|timer\|zsh\)\|qwagent-triage-install\|telegram\.yml" --include='*' . | grep -v '^./docs/superpowers' | grep -v '^./.git/'`
Expected: only the files listed above, `Makefile`, `deploy/README.md`, `docs/TELEGRAM_OPERATOR.md`, `deploy/monitoring/*`, and `deploy/fixer*` references to `qwagent-triage.sh` (keep `qwagent-triage.sh` and `qwagent-triage.prompt.md`: the Swarm fixer image uses them). If anything else references a file you are about to delete, stop and keep that file.

- [ ] **Step 2: Write the failing test** `deploy/ops_config_test.go`

```go
package deploy

import (
	"os"
	"strings"
	"testing"
)

// ops.yml must keep the bot unprivileged and give the socket only to watch.
func TestOpsStackPrivilegeBoundaries(t *testing.T) {
	raw, err := os.ReadFile("ops.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	parts := strings.Split(text, "\n  ")
	section := func(name string) string {
		for _, p := range parts {
			if strings.HasPrefix(p, name+":") {
				return p
			}
		}
		t.Fatalf("service %s missing", name)
		return ""
	}
	if strings.Contains(section("telegram"), "docker.sock") || strings.Contains(section("telegram"), "/host") {
		t.Fatal("telegram must not mount the Docker socket or the host")
	}
	if !strings.Contains(section("watch"), "/var/run/docker.sock") || !strings.Contains(section("watch"), "node.role == manager") {
		t.Fatal("watch needs the socket and a manager placement")
	}
	n := section("node")
	if !strings.Contains(n, "mode: global") || !strings.Contains(n, "/:/host:ro") {
		t.Fatal("node must be global with a read-only host root")
	}
	if !strings.Contains(text, "pokefarm_ops_token") {
		t.Fatal("shared ops token secret missing")
	}
}
```

Before writing it, check how other stack tests in `deploy/` parse compose files (`grep -ln "farm.yml\|fixer.yml" deploy/*_test.go`). If an existing test already parses YAML (for example with a helper or `gopkg.in/yaml` already in `go.mod`), use that helper instead of string splitting.

- [ ] **Step 3: Run** `go test ./deploy/ -run TestOpsStackPrivilegeBoundaries -count=1` → FAIL (no ops.yml).

- [ ] **Step 4: Write `deploy/ops.yml`**

```yaml
# Ops overlay: Telegram bot, Swarm watcher, per-node reporter. Deploy after
# deploy/farm.yml so the farm network exists:
#
#   printf %s "$TOKEN" | docker secret create poketelegram_bot_token -
#   openssl rand -hex 32 | docker secret create pokefarm_ops_token -
#   printf %s "$GH_READONLY_TOKEN" | docker secret create pokewatch_github_token -
#   docker stack deploy -c deploy/ops.yml pokefarm-ops
#
# Separate from farm.yml: a missing Telegram token must never block the farm.
# Only `watch` sees the Docker socket; `telegram` stays unprivileged.
services:
  telegram:
    image: ${FARM_IMAGE:-pokepilot-farm:local}
    command: ["poketelegram"]
    environment:
      TELEGRAM_BOT_TOKEN_FILE: /run/secrets/poketelegram_bot_token
      TELEGRAM_ALLOWED_USER_IDS: ${TELEGRAM_ALLOWED_USER_IDS:-}
      TELEGRAM_ALLOWED_CHAT_IDS: ${TELEGRAM_ALLOWED_CHAT_IDS:-}
      TELEGRAM_NOTIFY_CHAT_IDS: ${TELEGRAM_NOTIFY_CHAT_IDS:-}
      TELEGRAM_CHAT_ID: ${TELEGRAM_CHAT_ID:-}
      POKEPILOT_OPERATOR_URL: ${POKEPILOT_OPERATOR_URL:-http://wall:8080}
      POKEPILOT_REPLAY_URL: ${FARM_REPLAY_URL:-}
      POKEPILOT_ALERTMANAGER_URL: ${POKEPILOT_ALERTMANAGER_URL:-}
      POKEPILOT_ADMIN_BASE_URL: ${POKEPILOT_ADMIN_BASE_URL:-https://admin.rompilot.app}
      POKEPILOT_SPECTATOR_BASE_URL: ${POKEPILOT_SPECTATOR_BASE_URL:-https://rompilot.app/runs}
      POKEPILOT_GITHUB_REPO: ${POKEPILOT_GITHUB_REPO:-maestroi/PokePilot}
      POKEPILOT_OPS_TOKEN_FILE: /run/secrets/pokefarm_ops_token
      POKEPILOT_WATCH_DIGEST_HOUR: ${POKEPILOT_WATCH_DIGEST_HOUR:-9}
      POKETELEGRAM_STALL_AFTER: ${POKETELEGRAM_STALL_AFTER:-30m}
    secrets: [poketelegram_bot_token, pokefarm_ops_token]
    networks: [farm, ops]
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "-", "http://127.0.0.1:8080/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    deploy:
      replicas: 1
      restart_policy: {condition: any, delay: 5s}

  watch:
    image: ${FARM_IMAGE:-pokepilot-farm:local}
    command: ["pokewatch"]
    environment:
      POKEPILOT_OPS_TOKEN_FILE: /run/secrets/pokefarm_ops_token
      POKEPILOT_GITHUB_TOKEN_FILE: /run/secrets/pokewatch_github_token
      POKEPILOT_GITHUB_REPO: ${POKEPILOT_GITHUB_REPO:-maestroi/PokePilot}
      POKEWATCH_STACKS: ${POKEWATCH_STACKS:-pokefarm,pokefixer}
      POKEPILOT_WATCH_MIN_FREE_GB: ${POKEPILOT_WATCH_MIN_FREE_GB:-20}
      POKEPILOT_TELEGRAM_URL: http://telegram:8080
    secrets: [pokefarm_ops_token, pokewatch_github_token]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    networks: [ops]
    deploy:
      replicas: 1
      placement:
        constraints: [node.role == manager]
      restart_policy: {condition: any, delay: 10s}

  node:
    image: ${FARM_IMAGE:-pokepilot-farm:local}
    command: ["pokewatch", "-node"]
    environment:
      NODE_NAME: "{{.Node.Hostname}}"
      POKEPILOT_OPS_TOKEN_FILE: /run/secrets/pokefarm_ops_token
      POKEPILOT_FIXER_LEDGER: ${POKEPILOT_FIXER_LEDGER:-/opt/pokefixer/state/ledger.tsv}
      POKEPILOT_TRIAGE_LADDER: ${POKEPILOT_TRIAGE_LADDER:-opencode:2,cursor:2,cursor/claude-opus-5-5-high:2}
      POKEPILOT_PAID_DAILY_CAP: ${POKEPILOT_PAID_DAILY_CAP:-20}
      POKEWATCH_URL: http://watch:8080
    secrets: [pokefarm_ops_token]
    volumes:
      - /:/host:ro
    networks: [ops]
    deploy:
      mode: global
      resources:
        limits: {memory: 64M}
      restart_policy: {condition: any, delay: 10s}

networks:
  farm:
    external: true
    name: ${POKEPILOT_FARM_NETWORK:-pokefarm_default}
  ops:
    driver: overlay

secrets:
  poketelegram_bot_token:
    external: true
  pokefarm_ops_token:
    external: true
  pokewatch_github_token:
    external: true
```

Notes for the implementer:
- `POKETELEGRAM_STALL_AFTER` default moves to `30m` here to match the retired watchdog's 1800s stall threshold (the code default stays 15m).
- `TELEGRAM_BOT_TOKEN` env is intentionally absent: only the secret file is used.
- `deploy/Dockerfile`: next to the `poketelegram` build line add `CGO_ENABLED=0 go build -o /out/pokewatch ./cmd/pokewatch && \` and make sure `/out/pokewatch` lands in the same directory the other binaries are copied to (follow the existing `COPY --from=` line for `poketelegram`).

- [ ] **Step 5: Delete the retired files and update docs**

```bash
git rm deploy/telegram.yml deploy/farm-watch.sh deploy/farm-watch.service.in deploy/farm-watch.timer deploy/farm_watch_test.go deploy/qwagent-triage.service.in deploy/qwagent-triage.timer deploy/qwagent-triage.zsh
```

- `Makefile`: remove `qwagent-triage-install` from `.PHONY` and delete its recipe.
- `deploy/README.md`: replace the "Farm watchdog (Telegram)" section with a short "Ops (Telegram, watcher)" section pointing at `deploy/ops.yml` and `docs/TELEGRAM_OPERATOR.md`; remove the `make qwagent-triage-install` lines from "Local qwagent triage" (keep the parts describing `qwagent-triage.sh`, which the Swarm fixer runs).
- `docs/TELEGRAM_OPERATOR.md`: update the command table (menu, runs, run N, flag N, health, fixer, board, reply shortcuts), add "Alerts" (grace/remind/mute/restart summary), "Flag stuck" (what it does, the 10-minute fallback and that it files no issue when the runner never stops), "Watcher and node reporter" (checks list, freeze label), and replace the Swarm deployment section with the `deploy/ops.yml` commands above.
- `deploy/monitoring/*`: replace `pokefarm-telegram`/`tasks.telegram` stack references with `pokefarm-ops`.

- [ ] **Step 6: Run the full verification**

```bash
unset $(env | grep -o '^POKEPILOT_S3_[A-Z_]*') 2>/dev/null
make fmt-check
go vet ./...
go test -short -count=1 ./...
docker stack config -c deploy/ops.yml >/dev/null && echo ops-config-ok
```

Expected: all pass; `ops-config-ok` printed (requires the Docker CLI; skip that line only if Docker is unavailable and say so in the PR).

- [ ] **Step 7: Commit**

```bash
git add -A deploy Makefile docs
git commit -m "feat(deploy): pokefarm-ops stack replaces farm-watch and the desktop timers"
```

---

### Task 11: Roll out (operator-run, after merge)

Not code; recorded so nothing is forgotten. Run only after the PR merges and the image with `pokewatch` is published.

- [ ] On the manager: create secrets `pokefarm_ops_token` (`openssl rand -hex 32`) and `pokewatch_github_token` (fine-grained, read-only: pull requests + issues on the repo). `poketelegram_bot_token` already exists.
- [ ] Homelab `deploy/swarm.sh` (gitignored): rename `cmd_telegram` → `cmd_ops`; it resolves `deploy/ops.yml` with `FARM_IMAGE` = the wall's current digest, `POKEPILOT_FARM_NETWORK=pokefarm_internal`, `FARM_REPLAY_URL=http://192.168.50.203:8080`, `TELEGRAM_CHAT_ID` from `~/.config/pokepilot/env`; no secret-override file is needed any more because `ops.yml` declares the secrets.
- [ ] `docker stack rm pokefarm-telegram`, then `./deploy/swarm.sh ops`.
- [ ] Verify: `docker service ls --filter label=com.docker.stack.namespace=pokefarm-ops` shows telegram 1/1, watch 1/1, node N/N; within 2 minutes the bot's Health card lists every node, disk and service; `/board` posts a board; worker-04's manager alert appears (or the restart summary includes it).
- [ ] Verify the freeze label path once: `docker service update --label-add pokepilot.deploy-frozen-until=$(( $(date +%s) + 300 )) pokefarm_wall`, run `/usr/local/sbin/pokefarm-pull` and see "deploys frozen until", then remove the label.
