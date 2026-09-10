# Run Console Overhaul Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the private PokéFarm console a watching-first run workspace where an operator can understand the current game state, move through a recorded run, inspect failure evidence, and restart from a checkpoint without losing operational visibility.

**Architecture:** Keep the existing Go embed/proxy boundary and vanilla browser client. Replace the current watch-plus-detached-inspector page with one stateful console shell. `ui.js` owns dashboard selection, live frames, semantic map, tabs, and operations summaries; `inspector.js` owns selected-run debug data, checkpoints, replay transport, chronological Run Story, evidence, and run-scoped actions; `stats.js` renders aggregate outcomes only inside Operations. The timeline composes persisted API facts and never invents semantic events for unrecorded frames.

**Tech Stack:** Go standard library HTTP/embed, HTML, CSS, vanilla JavaScript, Canvas, HTML video, existing PokéFarm/PokeReplay APIs.

---

### Task 1: Pin the new console contract

**Files:**
- Modify: `cmd/pokeui/console_test.go`
- Modify: `cmd/pokeui/checkpoint_repro_ui_test.go`
- Test: `cmd/pokeui/console_test.go`
- Test: `cmd/pokeui/checkpoint_repro_ui_test.go`

**Step 1: Replace obsolete layout assertions with outcome-focused assertions**

Add checks for the durable regions and labels: `Live`, `Runs`, `Failures`, `Operations`, `Tools`, `run-rail`, `selected-run`, `visual-stage`, `run-timeline`, `run-story`, `system-summary`, and `evidence-drawer`. Remove assertions that require the inspector to live inside history or that require the old equal-height card grid.

**Step 2: Pin contextual action copy**

Update the checkpoint regression to require `Start a new run from here`, checkpoint eligibility text, `/checkpoints`, and `/repro`. Add assertions that failure evidence contains `Investigate with AI` and that replay controls contain `Generate replay` and `Return to live`.

**Step 3: Run the focused tests and confirm they fail**

Run: `go test ./cmd/pokeui -run 'TestUI|TestRunInspectorOffersCheckpointRepro'`

Expected: FAIL because the approved shell, Run Story, evidence drawer, and new action labels are not implemented.

### Task 2: Build the semantic shell and visual system

**Files:**
- Create: `cmd/pokeui/ui/console.css`
- Modify: `cmd/pokeui/ui/index.html`
- Modify: `cmd/pokeui/main.go`
- Modify: `cmd/pokeui/main_test.go`

**Step 1: Add a failing asset-serving test**

Extend the existing handler tests to request `/console.css`, require `200 OK`, `text/css`, and `Cache-Control: no-store`.

**Step 2: Add the embedded stylesheet**

Embed `ui/console.css` in `main.go` and serve it from `/console.css`. Keep the operator page assembly and script injection behavior unchanged.

**Step 3: Replace the page structure**

Rebuild `index.html` around:

- a compact global bar with health, connection state, tabs, and `New run`;
- a run source rail;
- a `Live` workspace with selected-run header, paired Game and Semantic Map stage, current-state strip, timeline, Run Story, and evidence drawer;
- secondary `Runs`, `Failures`, `Operations`, and `Tools` views;
- semantic buttons, headings, status regions, and an accessible mobile rail toggle.

Move the current inline presentation into `console.css`. Preserve existing IDs only where JavaScript still owns them, and add compatibility anchors where needed during the migration.

**Step 4: Implement the approved visual system**

Use fitted dark equipment bays, one-pixel separators, restrained corner radii, tabular machine data, cyan live/selection, amber checkpoints, green confirmed progress, and red failures. Give the Game and Semantic Map pair most of the first viewport. Add visible focus, reduced-motion behavior, loading skeletons, a narrow layout, and a rail drawer below desktop width.

**Step 5: Run the focused shell tests**

Run: `go test ./cmd/pokeui -run 'TestUI|TestOperator|TestIndex|TestConsole'`

Expected: PASS for the shell and asset-serving contracts; behavior tests may remain red until the next tasks.

### Task 3: Make selected-run state drive the workspace

**Files:**
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/console_test.go`

**Step 1: Add state-behavior contract tests**

Require a single console state with active tab, selected run, live-follow state, selected event, and dashboard freshness. Pin the selection priority: URL run ID, active run, last selected run, then most recent finished run. Require selection to update the URL without navigation.

**Step 2: Refactor rendering around the selected run**

Keep dashboard polling and the capped frame pump, but render the rail and workspace from one selected-run state. Active runs appear before recent runs. Each rail row shows status, preview, objective or goal, location, and elapsed/end time. Put the exact run ID once in the selected-run header with one copy action.

**Step 3: Add tab routing and compact operations state**

Implement keyboard-operable top tabs using URL hash state. `Live` retains a compact system summary with worker availability, queue size, active count, failure count, and data age. Move the large queue form and administrative lists out of the Live workspace.

**Step 4: Preserve honest connection behavior**

On a poll failure, keep the last selected run on screen, mark data stale, show its age, and stop presenting it as live. Restore live state after a successful poll.

**Step 5: Run focused tests**

Run: `go test ./cmd/pokeui -run 'TestUI(FramePump|DoesNotKeepPumping|Console|Selection|Tabs|Disconnect)'`

Expected: PASS.

### Task 4: Join the game, semantic map, state, and planner decision

**Files:**
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/ui/console.css`
- Modify: `cmd/pokeui/console_test.go`

**Step 1: Add view contract tests**

Pin the paired Game and Semantic Map region, adjacent current objective and latest AI decision, location/frame values, compact party state, badges, money, progress, and recorded-data unavailable labels.

**Step 2: Reuse frame and map rendering in the new stage**

Move the existing live `<img>` and canvas map into the paired stage. Preserve map caching, player/sprite/trail rendering, and the frame-rate cap. Make resize behavior derive from the rendered bay rather than fixed card dimensions.

**Step 3: Replace fragmented detail cards with one state strip**

Render Objective, Latest AI decision, Location, Frame, Party, and progress as a compact aligned region under the visual pair. Keep settings and detailed planner statistics available through evidence or Operations instead of duplicating them in the primary scan.

**Step 4: Run focused tests**

Run: `go test ./cmd/pokeui -run 'TestUI(FramePump|ShowsLatestPlan|RendersLLMStats|VisualStage|GameState)'`

Expected: PASS.

### Task 5: Build replay transport and chronological Run Story

**Files:**
- Modify: `cmd/pokeui/ui/inspector.js`
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/ui/console.css`
- Modify: `cmd/pokeui/console_test.go`
- Modify: `cmd/pokeui/checkpoint_repro_ui_test.go`

**Step 1: Add timeline ordering and checkpoint eligibility tests**

Extract pure browser helpers for normalizing debug timeline markers and checkpoints, sorting by frame/round/time, choosing stable marker kinds, and computing restart eligibility. Test them through source contracts and, where the repository's JavaScript test runner permits, direct helper execution.

**Step 2: Compose only persisted markers**

Load debug, artifacts, checkpoints, replay source/status, and dashboard state for the selected run. Merge recorded debug markers and checkpoint metadata into a stable ordered model. Label sparse history as `Recorded events`; show unavailable detail explicitly. Never infer battle, objective, or decision facts from video position.

**Step 3: Implement one transport model**

For active runs, default to live follow. Selecting or scrolling to an older marker pauses following and exposes `Return to live`. For completed runs, use replay video position as the visual playhead and snap semantic detail to the nearest persisted marker. Put `Generate replay` and generation state beside transport controls.

**Step 4: Render Run Story and synchronized selection**

Render chronological entries with frame/round, marker type, objective or decision text, outcome, state change, and location when present. Selecting a marker focuses and expands its Run Story entry; selecting an entry updates the timeline selection. Keep keyboard equivalents for every marker.

**Step 5: Put checkpoint restart at the selected marker**

Show `Start a new run from here` only for the selected checkpoint context. Include round/frame and paired-knowledge availability. Disable the action with the backend reason when the checkpoint is not replayable. POST the existing checkpoint name to `/v1/runs/{id}/repro` and select the new run returned by the API.

**Step 6: Run focused tests**

Run: `go test ./cmd/pokeui -run 'TestUI|TestRunInspectorOffersCheckpointRepro|TestRunInspectorProxy|TestReplay'`

Expected: PASS.

### Task 6: Integrate evidence and AI investigation

**Files:**
- Modify: `cmd/pokeui/ui/inspector.js`
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/ui/console.css`
- Modify: `cmd/pokeui/console_test.go`

**Step 1: Add action-placement tests**

Require `Investigate with AI` inside failure context, artifact links and raw debug content inside `evidence-drawer`, cancel in the active-run header, and delete in a run-level overflow control.

**Step 2: Build the evidence drawer**

Move raw planner exchanges, trace tail, artifact metadata/downloads, hashes, runner revision, model statistics, and debug JSON into one disclosure surface. Open it from the selected Run Story entry while preserving the selected run and marker.

**Step 3: Connect failure investigation**

Map a selected run failure to its triage group when available. Put `Investigate with AI` beside the structured failure summary, show the run ID and evidence scope that will be submitted, and call the existing triage investigation endpoint. Present accepted, already-open, and failed results in the same context.

**Step 4: Run focused tests**

Run: `go test ./cmd/pokeui -run 'TestUIFailuresAndIssueLinks|TestUIEvidence|TestUIInvestigate|TestRunInspector'`

Expected: PASS.

### Task 7: Finish the secondary admin views

**Files:**
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/ui/stats.js`
- Modify: `cmd/pokeui/ui/console.css`
- Modify: `cmd/pokeui/console_test.go`

**Step 1: Move existing functions into their owning tabs**

- `Runs`: searchable, paginated run archive with replay and outcome state.
- `Failures`: grouped actionable failures with representative-run deep links.
- `Operations`: workers, queue, service health, versions, and aggregate run outcomes.
- `Tools`: new-run form and secondary utilities.

Keep `New run` in the global bar as a direct route to Tools. Preserve cancel/delete confirmations and existing form/API payloads.

**Step 2: Refactor stats injection**

Render aggregate outcomes into the Operations target supplied by `index.html`; remove watch-pane injections that duplicate selected-run progress.

**Step 3: Run focused tests**

Run: `go test ./cmd/pokeui -run 'TestUIHistory|TestUIFailures|TestUIRendersLLMStats|TestUIOperations|TestUITools'`

Expected: PASS.

### Task 8: Verify the complete console

**Files:**
- Modify only if verification exposes a defect.

**Step 1: Format and run repository tests**

Run: `gofmt -w cmd/pokeui/*.go`

Run: `go test ./cmd/pokeui ./cmd/pokewall`

Run: `go test ./...`

Expected: PASS, except any unrelated journey failure must be reported from its dumped state rather than adopted as part of this UI change.

**Step 2: Run static UI checks**

Run the Impeccable deterministic hook for `cmd/pokeui/ui/index.html`, inspect the browser console, and confirm there are no JavaScript errors, missing assets, horizontal page overflow, or stale legacy inspector targets.

**Step 3: Perform browser scenarios**

Verify desktop and narrow layouts for: no active runs, one active run, multiple active runs, disconnected/stale data, a completed replayable run, and a failed run with checkpoint and AI investigation. Exercise keyboard tabs, rail selection, timeline markers, evidence disclosure, return-to-live, replay generation, and checkpoint restart.

**Step 4: Perform a live farm smoke test**

Against the configured PokéFarm services, confirm dashboard polling, live frame updates, semantic map updates, run deep links, replay status/video, and non-destructive inspection endpoints. Exercise mutating actions only with a disposable run created for this validation.

**Step 5: Clean generated files and review the diff**

Remove rejected mockups and generated cache files, preserve the unrelated `repro-run-3ray6e2s8w1j63np7mni08zgve/` directory, and confirm no ROM, `.sav`, or `.state` file is staged.

Run: `git status --short && git diff --check && git diff --stat`

Expected: only the console redesign, its tests, and approved design documents remain.
