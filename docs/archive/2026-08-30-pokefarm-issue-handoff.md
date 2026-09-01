# Pokéfarm Issue Handoff Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Automatically file every qualifying Pokéfarm failure with crash-safe replay evidence, deduplicate/count it in Agent Orchestrator, and keep its issue number and resolution synchronized in the farm console.

**Architecture:** The farm runner adds bounded objective checkpoints plus a periodic flight recorder and uploads recent checkpoints independently of heartbeats. The wall validates/durably stores evidence, groups failures by SHA-256 fingerprint, persists an automatic report outbox, and synchronizes linked issue status through a small Agent Orchestrator client; `pokeui` only renders status and proxies an optional threshold override. Agent Orchestrator remains the authority for numbering, occurrence counts, investigation, resolution, draft repair plans, and coding.

**Tech Stack:** Go 1.24, existing standard-library HTTP services, embedded HTML/CSS/JavaScript, Docker Swarm, Agent Orchestrator external issue API.

---

Prerequisites and constraints:

- Land Agent Orchestrator's `docs/archive/2026-08-30-external-issue-intake.md` first and verify its exact multipart response/route contract before implementation.
- Follow `docs/archive/2026-08-30-pokefarm-issue-handoff-design.md` and repository `AGENTS.md`.
- Never read a collision grid into context. Use `skill/probe_test.go` for any state/location question.
- Never commit `.gb`, `.sav`, `.state`, uploaded issue artifacts, or a ROM-derived fixture.
- Keep all Agent Orchestrator concepts out of `emu`, `skill`, `agent`, and `red`; integration belongs to `farm`, `cmd/pokepilot`, `cmd/pokewall`, and `cmd/pokeui`.
- Preserve the emulator single-goroutine invariant. Read checkpoint files only after `agent.Run` returns and stop/join heartbeat before final state/artifact collection.
- Under Agent Runner, do not commit. Leave edits uncommitted for the runner's verification/finalization gate.

### Task 1: Extend the farm Finish evidence contract

**Files:**
- Modify: `farm/spec.go`
- Modify: `farm/spec_test.go`
- Modify: `farm/client_test.go`

**Step 1: Write failing JSON round-trip tests**

Add tests proving Finish preserves:

- `runner_version`;
- `seed_burn` including zero;
- artifacts with `name`, `media_type`, `sha256`, and binary `data`;
- backward compatibility when older reports omit every new field.

Reject duplicate/empty artifact names, negative seed burn, mismatched SHA-256,
and a total artifact payload over a package constant chosen to remain below the
Agent Orchestrator report limit (24 MiB leaves multipart overhead below its
32 MiB default).

**Step 2: Run tests and verify failure**

Run: `go test ./farm -run 'Test.*Finish|TestArtifact' -count=1`

Expected: FAIL because Finish evidence fields and validation do not exist.

**Step 3: Add the wire types and validator**

Add:

```go
type Artifact struct {
    Name      string `json:"name"`
    MediaType string `json:"media_type"`
    SHA256    string `json:"sha256"`
    Data      []byte `json:"data"`
}
```

Extend `FinishReport` with `RunnerVersion`, `SeedBurn`, and `Artifacts`. Add
`ValidateFinishArtifacts` using `path.Base(name) == name`, conservative ASCII
names, exact lowercase hex hashes, and constant-time hash comparison. Validation
must not interpret `.state` contents.

**Step 4: Apply validation at the client boundary**

`farm.Client.Finish` rejects an invalid local report before sending HTTP. The
wall validates independently because runner input is still network input.

**Step 5: Run tests**

Run: `go test ./farm -count=1`

Expected: PASS.

**Step 6: Commit**

Human execution only:

```bash
git add farm/spec.go farm/spec_test.go farm/client_test.go
git commit -m "feat: carry replayable farm finish evidence"
```

### Task 2: Capture and upload a crash-safe rolling flight recorder

**Files:**
- Modify: `cmd/pokepilot/farm.go`
- Modify: `cmd/pokepilot/farm_test.go`
- Create: `cmd/pokepilot/farm_artifact_test.go`
- Modify: `farm/client.go`
- Modify: `farm/client_test.go`

**Step 1: Write failing artifact collection tests**

Use temporary directories containing representative `.state` and paired
`.knowledge-v*.json` files. Prove collection:

- sorts by checkpoint filename;
- keeps each state beside its knowledge file;
- rejects orphan knowledge, missing pairs, unsafe names, hash mismatch, and
  over-budget content;
- never includes unrelated files;
- produces deterministic names and hashes;
- removes the temporary directory only after the Finish call returns.
- captures periodic states every 18,000 frames and retains only 12;
- sends copied bytes through a capacity-one queue while the uploader never touches `*emu.Emu`;
- uploads new objective pairs and periodic states through a bounded checkpoint request;
- a blocked checkpoint endpoint cannot block gameplay or heartbeat cancellation.

Add a ROM-backed focused test proving `SaveState` invoked from the existing
sample callback can be loaded and resumes from the captured frame/map/tile.
This is a safety measurement, not a journey outcome. If it fails, stop this task
and add a deliberate safe frame-boundary capture hook before continuing.

Add a source-level/behavior test proving farm LLM budget receives a non-empty
`CheckpointDir` while scripted mode does not manufacture checkpoints.

**Step 2: Run tests and verify failure**

Run: `go test ./cmd/pokepilot -run 'TestFarm.*Artifact|TestRunFarmLLM.*Checkpoint' -count=1`

Expected: FAIL because leased runs do not create checkpoint directories or
collect evidence.

After adding the callback round-trip test, run its required measurement with:

```bash
POKEMON_RED_ROM=roms/pokemon_red.gb go test ./cmd/pokepilot -run '^TestFarmPeriodicCheckpointRoundTrip$' -count=1 -v
```

Expected: PASS with the loaded checkpoint at the captured frame/map/tile. Do
not use a stochastic journey or battle outcome as this safety gate.

**Step 3: Create one checkpoint directory per lease**

In `runOne`, create a `os.MkdirTemp("", "pokefarm-checkpoints-")` directory
only for LLM runs. Pass it into `runFarmLLM`, then into `agent.Budget.CheckpointDir`.
Use the existing default ring size of 16; do not introduce a second eviction
policy.

**Step 4: Add the periodic recorder and checkpoint uploader**

Compose periodic capture into the existing stepping-thread `OnSample` callback.
At the interval, copy a completed save state plus plain metadata into a
capacity-one channel. A single uploader goroutine hashes/writes/uploads it with
a finite context and drops superseded queued periodic samples rather than
blocking gameplay. A filesystem watcher/uploader sends newly completed
objective state/knowledge pairs only after both files exist.

Add `farm.Client.Checkpoint` for `POST /v1/runs/{id}/checkpoint`. It accepts
plain artifact bytes/metadata and never receives `*emu.Emu`.

**Step 5: Collect Finish evidence after gameplay**

After stopping and joining heartbeat, read the checkpoint directory into
`[]farm.Artifact`, add runner `version` and the already resolved `burn`, and
pass them to `finishRun`. The final state remains `FinishReport.SaveState`, not
a duplicate artifact.

Use `defer os.RemoveAll(checkpointDir)` with a validated non-empty temp path.
Do not remove before `client.Finish` returns.

**Step 6: Run tests**

Run: `go test ./cmd/pokepilot -count=1`

Expected: PASS.

**Step 7: Commit**

Human execution only:

```bash
git add cmd/pokepilot/farm.go cmd/pokepilot/farm_test.go cmd/pokepilot/farm_artifact_test.go farm/client.go farm/client_test.go
git commit -m "feat: checkpoint leased farm runs"
```

### Task 3: Make wall failure groups issue-ready and persistent

**Files:**
- Modify: `cmd/pokewall/wall.go`
- Modify: `cmd/pokewall/wall_test.go`
- Modify: `cmd/pokewall/triage_test.go`
- Modify: `cmd/pokewall/state_test.go`
- Modify: `cmd/pokewall/dashboard_test.go`

**Step 1: Write failing fingerprint and validation tests**

Prove:

- the same normalized pattern always receives the same 16-character key and
  full `sha256:<64 hex>` fingerprint;
- coordinates/map IDs/frame numbers normalize together while different words
  remain different groups;
- triage selects newest finished representative first;
- Finish rejects corrupt/oversized artifacts before settling or writing a dump;
- wall persistence round-trips issue UUID, number, URL, last reported run, and
  timestamp by failure key;
- wall persistence round-trips pending/completed report outbox entries and last synchronized issue status;
- the checkpoint route retains only the latest three periodic states and latest complete objective pair per run;
- dashboard/triage JSON expose the issue link without exposing artifact bytes.

**Step 2: Run tests and verify failure**

Run: `go test ./cmd/pokewall -run 'TestTriage|TestFinishArtifact|TestIssueLink|TestDashboard' -count=1`

Expected: FAIL because groups have no stable key/link and Finish does not
validate evidence.

**Step 3: Add stable group identity**

Extend `triageGroup` with `Key`, `Fingerprint`, and optional plain-value
`IssueLink`. Compute SHA-256 from the complete normalized pattern and use only
the short key in Pokéfarm route paths. Iterate finished runs newest-first when
selecting representative IDs; keep count exact and sample bounded.

**Step 4: Persist issue links**

Add `IssueLink` and `map[string]IssueLink` to `Wall`/`persistedState`. Copy maps
under `w.mu`, as with tiles; never render live pointers after unlock. Old state
without links loads as an empty map.

Add a persisted idempotent report outbox keyed by external occurrence ID. A
qualifying `error|lost` settlement with non-empty detail is enqueued only after
its durable dump exists. Clean completion, cancellation, and budget stops never
enqueue. Store checkpoint artifacts below the existing safe per-run dump root;
never in wall JSON state.

Register `POST /v1/runs/{id}/checkpoint`. Validate run ID, attempt, artifact
name/hash/size, and accept only the currently leased/running attempt. Write to a
temporary file and atomically rename before acknowledging. Retain the latest
three periodic states and latest complete objective state/knowledge pair; reject
late uploads after Finish. Filesystem I/O happens outside `w.mu`, followed by a
locked attempt recheck before the rename becomes visible.

When snapshotting run rows, derive their failure key from final reason/detail
and attach the matching link. Do not alter clean/done runs.

**Step 5: Validate Finish before mutation**

Call `farm.ValidateFinishArtifacts` before fetching a frame, settling a tile,
or writing the dump. Preserve identical retry/idempotency behavior and
attempt-specific dump names.

**Step 6: Run tests**

Run: `go test -race ./cmd/pokewall -count=1`

Expected: PASS with no race reports.

**Step 7: Commit**

Human execution only:

```bash
git add cmd/pokewall/wall.go cmd/pokewall/wall_test.go cmd/pokewall/triage_test.go cmd/pokewall/state_test.go cmd/pokewall/dashboard_test.go
git commit -m "feat: identify and link farm failure groups"
```

### Task 4: Dispatch automatic reports and synchronize issue status

**Files:**
- Create: `cmd/pokewall/issues.go`
- Create: `cmd/pokewall/issues_test.go`
- Modify: `cmd/pokewall/wall.go`
- Modify: `cmd/pokewall/main.go`

**Step 1: Write failing client/report tests**

With `httptest.Server`, prove the wall:

- reads the newest representative's exact attempt dump using existing safe
  naming rules;
- creates source `pokefarm`, stable fingerprint, external ID
  `<run-id>-attempt-<n>`, observed runner revision, and complete structured
  evidence;
- streams `final.state`, final PNG when present, checkpoint states/knowledge,
  and a small finish manifest as multipart parts;
- validates artifact hashes before any network request;
- decodes issue UUID/number/status and constructs a UUID-backed UI link;
- accepts captured-only, auto-investigation-started, and automation-failed report responses without losing the issue link;
- retries timeout/unreachable responses from the durable outbox using the same external occurrence ID;
- fetches linked issue status and preserves the last known value on sync failure;
- performs file/network work without holding `w.mu`.

**Step 2: Run tests and verify failure**

Run: `go test ./cmd/pokewall -run 'TestIssueReport|TestInvestigateFailure' -count=1`

Expected: FAIL because no Agent Orchestrator client, outbox dispatcher, or status synchronizer exists.

**Step 3: Implement a bounded client**

Add an `issueClient` configured by API base, project UUID, UI base, and timeout.
Use `io.Pipe` plus `multipart.Writer` so the report is streamed rather than
buffered a second time. Close the pipe with the producer error, bound response
bodies, and treat non-2xx JSON `{error}` as upstream errors.

Use the exact Agent Orchestrator contract that landed; do not preserve guessed
field names from this plan when live API types differ.

**Step 4: Add wall orchestration**

Run a background outbox dispatcher with bounded exponential backoff. Under the
mutex, claim one immutable pending entry; release the mutex before dump I/O and
HTTP. On success, persist returned link/status and mark the occurrence complete.
On retryable failure, persist next-attempt time/error. Permanent validation or
configuration errors remain visible and manually retryable.

Register `POST /v1/triage/{key}/investigate` only as an **Investigate now**
threshold override for an already linked issue. It never creates or reports an
occurrence.

Add a low-frequency status synchronizer (default 30 seconds) for unique linked
issue UUIDs. Update status, resolution, occurrence count, and fixed revision in
plain `IssueLink` values without holding `w.mu` during HTTP. A later local
occurrence is submitted even when cached status says fixed; Agent Orchestrator
owns reopening.

**Step 5: Wire flags**

Add `-issues-api`, `-issues-project`, `-issues-ui`, and `-issues-timeout` to
`cmd/pokewall/main.go`. Enable integration only when all three required values
are non-empty; partial configuration is a startup error.

**Step 6: Run tests**

Run: `go test -race ./cmd/pokewall -count=1`

Expected: PASS with no race reports.

**Step 7: Commit**

Human execution only:

```bash
git add cmd/pokewall/issues.go cmd/pokewall/issues_test.go cmd/pokewall/wall.go cmd/pokewall/main.go
git commit -m "feat: submit farm failures for investigation"
```

### Task 5: Add Investigate and issue links to the console

**Files:**
- Modify: `cmd/pokeui/main.go`
- Modify: `cmd/pokeui/relay_test.go`
- Modify: `cmd/pokeui/ui/index.html`
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/console_test.go`

**Step 1: Write failing relay and UI tests**

Prove:

- `GET /v1/triage` and `POST /v1/triage/{key}/investigate` are proxied;
- other `/v1/triage/...` methods and every arbitrary Agent Orchestrator path
  remain 404;
- integration-disabled groups show local counts without issue controls;
- unreported groups show pending/error outbox state rather than a filing button;
- linked open groups below threshold show Investigate now and one in-flight click disables it;
- linked groups/history rows/details render escaped `Issue #42` anchors, status, occurrence count, and fixed revision using only the URL returned by the wall;
- resolved fixed groups leave active failures but remain filterable in history;
- polling patches the DOM without losing selected run, scroll, or in-flight
  button state.

**Step 2: Run tests and verify failure**

Run: `go test ./cmd/pokeui -run 'Test.*Triage|Test.*Issue|TestConsole' -count=1`

Expected: FAIL because the routes and failure UI do not exist.

**Step 3: Extend the strict proxy allowlist**

Add only the two specified wall routes. Preserve bounded proxy timeout,
`Cache-Control: no-store` on reads, upstream JSON content type, and 502 mapping.

**Step 4: Render failure groups**

Fetch triage alongside dashboard polling. Add a Failures section below live
runs and above history. Use `textContent`/the existing `esc` helper for all
patterns, examples, IDs, warnings, and labels. Validate issue URLs with
`new URL` and allow only `http:`/`https:` before assigning `href`.

Add issue badges/status to matching history rows and the detail pane. The issue number
is presentation; never build a controller URL from it in JavaScript.

**Step 5: Run tests**

Run: `go test ./cmd/pokeui -count=1`

Expected: PASS.

**Step 6: Commit**

Human execution only:

```bash
git add cmd/pokeui/main.go cmd/pokeui/relay_test.go cmd/pokeui/ui/index.html cmd/pokeui/ui/ui.js cmd/pokeui/console_test.go
git commit -m "feat: investigate failures from pokefarm"
```

### Task 6: Configure the Swarm integration without changing local defaults

**Files:**
- Modify: `deploy/farm.yml`
- Modify: `deploy/README.md`
- Modify: `Makefile`

**Step 1: Write the configuration assertions**

Add a small shell/Go assertion using the repository's existing deployment test
pattern, or a focused source test if no deploy parser exists, proving all three
issue settings reach only the wall service and no value is baked into the
image. Empty values must leave integration disabled.

**Step 2: Run the focused check and verify failure**

Run: `env -u POKEMON_RED_ROM go test ./... -run 'Test.*IssueConfig' -count=1`

Expected: FAIL because the stack does not pass issue integration settings.

**Step 3: Add environment-backed wall arguments**

Pass:

```yaml
-issues-api ${AGENT_ORCHESTRATOR_API:-}
-issues-project ${AGENT_ORCHESTRATOR_POKEPILOT_PROJECT_ID:-}
-issues-ui ${AGENT_ORCHESTRATOR_UI:-}
```

Use a command form that does not pass a partially configured triple. If Swarm
interpolation cannot express that safely, read the three values from wall
environment instead and validate them in Go. Do not add secrets or publish the
wall port.

**Step 4: Document exact configuration and reachability**

Document the current examples `http://192.168.50.81:8080` and
`http://192.168.50.81:8081` as operator-provided LAN values, not hard-coded
defaults. Include a wall-container `curl /api/health` reachability check and the
Agent Orchestrator project UUID lookup step.

**Step 5: Run ROM-free repository verification**

Run: `env -u POKEMON_RED_ROM go test ./... -count=1`

Expected: PASS.

**Step 6: Commit**

Human execution only:

```bash
git add deploy/farm.yml deploy/README.md Makefile
git commit -m "docs: configure pokefarm issue handoff"
```

### Task 7: Add an end-to-end issue handoff gate

**Files:**
- Create: `cmd/pokewall/issues_e2e_test.go`
- Modify: `cmd/pokewall/client_e2e_test.go`
- Modify: `docs/AGENT.md`

**Step 1: Write the end-to-end test**

Run a real Wall handler and a fake Agent Orchestrator HTTP server. Enqueue,
lease, heartbeat, and finish a representative failed run with:

- runner version and seed burn;
- final state;
- two checkpoint state/knowledge pairs;
- trace and last plan/decision.

Let the automatic outbox dispatch and assert the fake controller receives one
valid multipart report with correct SHA-256 values and no ROM. Return issue UUID,
number 42, status, and occurrence count, then prove:

- triage and dashboard show `#42` and the UUID-backed link;
- retrying the same outbox item makes no duplicate occurrence;
- wall state reload preserves the link;
- a second matching failed run automatically becomes occurrence 2 on the same issue;
- status sync moves fixed issues out of active failures and shows the fixed SHA;
- a later matching run is still submitted and reopens the same issue number;
- the original finish dump remains decodable and complete.

**Step 2: Add corruption and partial-success controls**

Prove corrupt evidence makes zero upstream calls and leaves a visible permanent
outbox error. Prove transient upstream failure survives state reload and retries
the same idempotent occurrence rather than creating another.

**Step 3: Document the investigation workflow**

Update `docs/AGENT.md` with the farm evidence fields and the operator handoff.
Make clear that a linked issue is not proof of a PokePilot defect: Agent
Orchestrator may classify it as expected game/RNG behavior or external
infrastructure.

**Step 4: Run focused race verification**

Run:

```bash
env -u POKEMON_RED_ROM go test -race ./farm ./cmd/pokepilot ./cmd/pokewall ./cmd/pokeui -count=1
```

Expected: PASS with no race reports.

**Step 5: Run full ROM-free verification**

Run:

```bash
env -u POKEMON_RED_ROM go test ./... -count=1
go build -o /tmp/pokepilot-issue-handoff ./cmd/pokepilot
go build -o /tmp/pokewall-issue-handoff ./cmd/pokewall
go build -o /tmp/pokeui-issue-handoff ./cmd/pokeui
git status --short
```

Expected: all tests/builds PASS. `git status --short` contains only intended
source/docs changes and no `.state`, `.gb`, `.sav`, checkpoint directory,
uploaded artifact, or repository-root binary.

**Step 6: Commit**

Human execution only:

```bash
git add cmd/pokewall/issues_e2e_test.go cmd/pokewall/client_e2e_test.go docs/AGENT.md
git commit -m "test: verify pokefarm issue handoff"
```
