# Game-Native Operator Console Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Turn the existing operator shell into the approved game-native console, place finished-run replay inside the Game bay, and separate fleet Operations from campaign Analytics.

**Architecture:** Keep the Go embed/proxy boundary and framework-free browser client. `ui.js` continues to own dashboard state, run selection, live frames, map rendering, tabs, and fleet rendering; `inspector.js` owns selected-run events, replay state, checkpoint actions, and evidence, but mounts replay into the Game bay; `stats.js` renders aggregate campaign statistics only in Analytics. Existing APIs remain unchanged.

**Tech Stack:** Go standard library HTTP/embed, HTML, CSS, vanilla JavaScript, Canvas, HTML video, existing PokéFarm/PokeReplay APIs.

---

### Task 1: Pin the revised information architecture

**Files:**
- Modify: `cmd/pokeui/console_test.go`
- Test: `cmd/pokeui/console_test.go`

**Step 1: Write failing navigation and ownership tests**

Extend `TestUIRunConsoleHasWatchingFirstWorkspace` to require:

```go
for _, want := range []string{
    `data-view="analytics"`,
    `data-console-view="analytics"`,
    `id="analytics-outcomes"`,
    `id="operations-health"`,
    `id="operations-workers"`,
    `id="operations-queue"`,
    `id="operations-recent"`,
} {
    if !strings.Contains(html, want) {
        t.Errorf("operator console missing %q", want)
    }
}
```

Add `TestUIReplayLivesInGameBay`:

```go
func TestUIReplayLivesInGameBay(t *testing.T) {
    html := string(indexHTML)
    inspector := string(inspectorJS)
    if !strings.Contains(html, `id="detail-game-media"`) {
        t.Fatal("Game bay must expose one live/replay media host")
    }
    if !strings.Contains(inspector, `detail-game-media`) {
        t.Error("inspector must mount replay into the Game bay")
    }
    if strings.Contains(inspector, `id="pp-video"`) {
        t.Error("inspector must not create a second detached replay video")
    }
}
```

Add `TestUIStatsBelongToAnalytics` requiring `stats.js` to target
`analytics-outcomes` and forbidding `run-outcomes`.

**Step 2: Run tests and verify failure**

Run:

```bash
go test ./cmd/pokeui -run 'TestUI(RunConsoleHasWatchingFirstWorkspace|ReplayLivesInGameBay|StatsBelongToAnalytics)'
```

Expected: FAIL because Analytics, fleet regions, and the shared Game media host
do not exist yet.

**Step 3: Commit the red tests**

```bash
git add cmd/pokeui/console_test.go
git commit -m "test: pin game-native console structure"
```

### Task 2: Reshape the console markup and navigation

**Files:**
- Modify: `cmd/pokeui/ui/index.html`
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/console_test.go`

**Step 1: Add the Analytics tab**

Place Analytics between Failures and Operations:

```html
<button type="button" role="tab" aria-selected="false" data-view="analytics">
  Analytics
</button>
```

Extend `setView`:

```js
const valid = ["live", "runs", "failures", "analytics", "operations", "tools"];
```

**Step 2: Create one Game media host**

Replace the Game monitor’s `detail-lcd` wrapper with:

```html
<div class="game-media" id="detail-game-media">
  <div class="lcd" id="detail-lcd">
    <span class="idle">Waiting for a frame</span>
  </div>
</div>
```

The host must remain stable while `ui.js` repaints live frames and
`inspector.js` swaps finished-run replay states.

**Step 3: Add a compact game-state bay**

Add `detail-game-state` as the third desktop column in `visual-stage`. Move
party, money, and badges into that bay while preserving the current
`partyHTML(run)` data source and all six party slots.

Keep the strip below the visual row limited to:

- objective;
- latest decision or final outcome;
- location;
- frame and round/time.

Settings, model statistics, and raw trace remain available in evidence.

**Step 4: Split Operations and Analytics markup**

Replace the current Operations body with:

```html
<div class="operations-grid">
  <section id="operations-health" class="ops-bay"></section>
  <section id="operations-workers" class="ops-bay">
    <header><h3>Workers</h3><p id="worker-summary"></p></header>
    <div id="workers"></div>
  </section>
  <section id="operations-queue" class="ops-bay"></section>
  <section id="operations-recent" class="ops-bay"></section>
</div>
```

Add:

```html
<section id="view-analytics" class="console-view"
         data-console-view="analytics" hidden>
  <header class="view-heading">
    <h2>Analytics</h2>
    <p>Campaign progress, outcomes, and endless experiments.</p>
  </header>
  <section id="analytics-outcomes"></section>
</section>
```

**Step 5: Run focused tests**

Run:

```bash
go test ./cmd/pokeui -run 'TestUI(RunConsole|Watch|RendersPlayerRoster|StatsBelongToAnalytics)'
```

Expected: PASS.

**Step 6: Commit**

```bash
git add cmd/pokeui/ui/index.html cmd/pokeui/ui/ui.js cmd/pokeui/console_test.go
git commit -m "feat: reshape operator console workspace"
```

### Task 3: Put finished replay inside the Game bay

**Files:**
- Modify: `cmd/pokeui/ui/inspector.js`
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/ui/console.css`
- Modify: `cmd/pokeui/console_test.go`
- Test: `cmd/pokeui/console_test.go`

**Step 1: Add replay-state contract tests**

Require the inspector source to contain the five states:

```go
for _, want := range []string{
    `"missing"`, `"generating"`, `"ready"`, `"disabled"`, `"error"`,
    `id="pp-game-video"`,
    `id="pp-game-replay-status"`,
    `nearest persisted event`,
} {
    if !strings.Contains(inspector, want) {
        t.Errorf("unified Game replay missing %q", want)
    }
}
```

Require `ui.js` to dispatch a run-state event after rendering the selected run
so the inspector can restore the live image when switching from a finished run.

**Step 2: Remove detached video markup**

Delete `pp-video` and `pp-scrubber-wrap` from the timeline template. The timeline
keeps event markers, legend, `Return to live`, and replay status text, but it no
longer owns visual media.

**Step 3: Render replay states in `detail-game-media`**

Create a small renderer that replaces only content owned by the inspector:

```js
function renderGameReplay(status) {
  const host = byID("detail-game-media");
  if (!host || !finishedRun) return;
  // missing/error: retain last frame and add local action/status overlay
  // generating: retain last frame and add progress status
  // ready: mount #pp-game-video and point it at /replay/video
}
```

Use an overlay sibling rather than replacing `detail-lcd` for missing,
generating, disabled, and error states. In the ready state, hide `detail-lcd`
and show:

```html
<video id="pp-game-video" controls preload="metadata"></video>
```

When selecting an active run, remove replay-owned elements, unhide
`detail-lcd`, and allow the capped frame pump to resume.

**Step 4: Synchronize timeline and video**

Preserve event-to-video seeking:

```js
if (video && video.duration > 0 && maxFrame > 0) {
  video.currentTime = video.duration * eventFrame(events[index]) / maxFrame;
}
```

On arbitrary video seeking, select the nearest persisted event by normalized
frame position. Label map/detail context:

`Semantic state from nearest persisted event`

Do not repaint the map as though it were an exact snapshot when no historical
map payload exists.

**Step 5: Keep generation controls local**

Place `Generate replay`, `Generating…`, `Retry replay`, and unavailable reasons
inside the Game bay overlay. The timeline may show transport status but must not
duplicate the action.

**Step 6: Run focused tests**

Run:

```bash
go test ./cmd/pokeui -run 'TestUI(Replay|FramePump|DoesNotKeepPumping|Inspector)'
```

Expected: PASS.

**Step 7: Commit**

```bash
git add cmd/pokeui/ui/inspector.js cmd/pokeui/ui/ui.js cmd/pokeui/ui/console.css cmd/pokeui/console_test.go
git commit -m "feat: unify live and replay game media"
```

### Task 4: Rebuild Operations as fleet control and move stats

**Files:**
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/ui/stats.js`
- Modify: `cmd/pokeui/ui/console.css`
- Modify: `cmd/pokeui/console_test.go`

**Step 1: Add fleet-content tests**

Require `ui.js` to render:

- connection/service health;
- each worker’s address, revision, last-seen age, and current run;
- queued and leased runs;
- running attempts;
- the most recent terminal outcomes with `data-run` deep links.

Forbid campaign terms such as `Badge distribution` inside the Operations
section of `index.html`.

**Step 2: Render fleet health**

Build `renderOperations()` from the existing dashboard snapshot:

```js
function renderOperations() {
  renderOperationsHealth(snap);
  renderOperationsQueue(snap.runs || []);
  renderOperationsRecent(snap.runs || []);
}
```

Use structured rows rather than equal KPI cards. Selecting a current assignment
or recent outcome should call `selectRun(id)` and return to Live.

Do not invent service status the dashboard does not provide. Show known
connection, workers, versions, and freshness; label unavailable service detail
as unavailable.

**Step 3: Move aggregate stats to Analytics**

Change `stats.js` to mount one coherent analytics surface into
`analytics-outcomes`. Preserve:

- completed attempts;
- objective wins;
- badge distribution;
- terminal outcomes;
- retry failures;
- endless-experiment table.

Remove the dynamic CSS injection from `stats.js`; move those styles into
`console.css`. Remove the additive `live-goal-progress` polling layer and render
selected-run goal progress from the main dashboard pass in `ui.js`.

**Step 4: Run focused tests**

Run:

```bash
go test ./cmd/pokeui -run 'TestUI(Operations|Stats|History|Failures|ShowsLatestPlan)'
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/pokeui/ui/ui.js cmd/pokeui/ui/stats.js cmd/pokeui/ui/console.css cmd/pokeui/console_test.go
git commit -m "feat: separate fleet operations from analytics"
```

### Task 5: Replace the generic dashboard theme with the approved visual system

**Files:**
- Modify: `DESIGN.md`
- Modify: `.impeccable/surfaces/cmd-pokeui-ui-index-html.md`
- Modify: `cmd/pokeui/ui/console.css`
- Modify: `cmd/pokeui/ui/index.html`

**Step 1: Record the durable visual contract**

Update `DESIGN.md` and the surface brief to state:

- game-native identity comes from game and semantic assets, not retro chrome;
- near-black/blue-black shell with fine cool rules;
- cyan selection/action, green progress, amber checkpoint, red failure;
- dense fitted bays and aligned tables;
- finished replay replaces live media in Game;
- Operations is fleet control and Analytics owns aggregates.

Update the opening direction comment in `index.html` to match.

**Step 2: Consolidate CSS**

Rewrite `console.css` into ordered sections:

1. tokens and reset;
2. global shell/navigation;
3. run rail;
4. selected-run visual workspace;
5. state strip and party;
6. timeline and Run Story;
7. evidence;
8. Runs/Failures;
9. Operations/Analytics/Tools;
10. responsive and reduced-motion rules.

Remove the appended “Watching-first reference alignment” and “Regression fixes”
override layers after their required behavior is represented in the owning
rules.

**Step 3: Match the approved references**

Desktop target:

- approximately 190–220px run rail;
- dense 36–44px global bar;
- Game and map dominate the first row;
- game-state bay remains compact;
- no large unused canvas below short Operations content;
- selected run/event uses a thin cyan edge or field;
- Pokémon game/map imagery supplies the strongest color.

Do not add generated artwork. Use only real frames, map assets, party data, and
existing status symbols.

**Step 4: Run UI detector once**

Run:

```bash
node /home/maestro/.cursor/skills/impeccable/scripts/detect.mjs --json \
  cmd/pokeui/ui/index.html \
  cmd/pokeui/ui/console.css \
  cmd/pokeui/ui/ui.js \
  cmd/pokeui/ui/inspector.js \
  cmd/pokeui/ui/stats.js
```

Expected: no material design-floor violations. Fix actionable findings without
changing the approved direction.

**Step 5: Commit**

```bash
git add DESIGN.md .impeccable/surfaces/cmd-pokeui-ui-index-html.md \
  cmd/pokeui/ui/index.html cmd/pokeui/ui/console.css
git commit -m "style: apply game-native operator console system"
```

### Task 6: Verify the finished console

**Files:**
- Modify only if verification reveals an in-scope defect.

**Step 1: Run focused packages**

Run:

```bash
go test ./cmd/pokeui ./cmd/pokewall
```

Expected: PASS.

**Step 2: Run the repository suite**

Run:

```bash
go test ./...
```

Expected: PASS. Report unrelated stochastic journey failures from their dumped
state rather than adopting them into this UI task.

**Step 3: Inspect browser scenarios**

At desktop and narrow widths verify:

- one active run with live frame updates;
- a finished run with missing replay;
- replay generation in progress;
- ready replay replacing the Game image;
- failed replay generation and retry;
- failed run with expanded evidence and investigation;
- checkpoint restart;
- disconnected/stale dashboard;
- fleet Operations;
- campaign Analytics.

Confirm keyboard tabs, run rows, timeline markers, Run Story rows, evidence, and
actions all expose visible focus.

**Step 4: Review the diff**

Run:

```bash
git status --short
git diff --check
git diff --stat
```

Expected: only the approved console redesign and its tests are changed. Preserve
the unrelated untracked `repro-run-3ray6e2s8w1j63np7mni08zgve/` directory and
do not stage generated mockups, ROMs, saves, or states.
