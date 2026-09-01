# LLM Stats in Poke UI — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show the runner's LLM planner tally (the panel on port 8099) in the poke ui console: a compact line on each live llm run card and the full panel in the detail pane.

**Architecture:** The tally already exists in `cmd/pokepilot/stats.go` (`runStats`, pushed to the runner's own watch page). It now also rides the existing 1 Hz heartbeat to the wall (typed as `farm.LLMStats`, the same JSON keys), is stored on the wall's tile (persisted, kept on finish, reset on retry), and is served in `/v1/dashboard` where pokeui renders it. No new endpoints, no new dependencies.

**Tech Stack:** Go stdlib only; embedded HTML/JS console (`cmd/pokeui/ui`).

**Design doc:** `docs/archive/2026-08-30-llm-stats-in-pokeui-design.md`

**Working-tree note:** the tree carries unrelated uncommitted work (agent/, emu/, cmd/pokepilot/farm.go). Every commit below lists exact file paths — never `git add -A`. If a test failure is clearly in files this plan does not touch, report it, do not adopt it.

---

### Task 1: Wire type — `farm.LLMStats` on the heartbeat

**Files:**
- Modify: `farm/spec.go` (new `LLMStats` + `ChoiceCount`; `Heartbeat.Stats`)
- Test: `farm/spec_test.go`

**Step 1: Write the failing test**

Append to `farm/spec_test.go`:

```go
func TestHeartbeatCarriesLLMStats(t *testing.T) {
	want := Heartbeat{
		RunID: "r1", Frame: 100,
		Stats: &LLMStats{
			Round: 3, RoundsLeft: 29, Calls: 4, Rounds: 3, Rejected: 1, Repeats: 1,
			AvgOffered: 5.5, LastSeconds: 4.4, AvgSeconds: 3.1,
			PromptTokens: 947, CompletionTokens: 36,
			Intent: "get a move on the badge", IntentAge: 2,
			Choices: []ChoiceCount{{Objective: "go to pallet town", Count: 2}},
		},
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Heartbeat
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Stats == nil || *got.Stats != *want.Stats {
		t.Fatalf("stats round trip = %+v, want %+v", got.Stats, want.Stats)
	}
	for _, field := range []string{`"stats"`, `"rounds_left"`, `"avg_offered"`, `"prompt_tokens"`, `"choices"`, `"objective"`} {
		if !contains(string(b), field) {
			t.Errorf("marshaled heartbeat missing %s: %s", field, b)
		}
	}

	// A scripted run (no stats) must marshal without the key at all.
	b, err = json.Marshal(Heartbeat{RunID: "r2"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(string(b), `"stats"`) {
		t.Errorf("nil stats must be omitted: %s", b)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./farm -run TestHeartbeatCarriesLLMStats`
Expected: FAIL — `undefined: LLMStats` (compile error).

**Step 3: Write minimal implementation**

In `farm/spec.go`, add to the `Heartbeat` struct (after the `Version` field):

```go
	// Stats is the llm planner's tally — the same numbers the runner's own
	// watch page renders, pushed here so the console shows them too. Nil on
	// scripted runs and on runners that predate it.
	Stats *LLMStats `json:"stats,omitempty"`
```

And add the two types after the `Heartbeat` struct:

```go
// LLMStats is the planner tally a runner pushes on its heartbeats: round
// progress, how often it re-picks an objective it already picked (Repeats —
// the wander signal), think time, spend, and the replies that never
// resolved. The wall carries it verbatim for the console; the field names
// are the same JSON keys the runner's watch page renders, so both surfaces
// show one number.
type LLMStats struct {
	Round      int `json:"round"`
	RoundsLeft int `json:"rounds_left"`
	// Calls counts every ask, Rounds only the ones that became an
	// objective: the gap between them is re-asks after a rejected reply.
	Calls    int `json:"calls"`
	Rounds   int `json:"rounds"`
	Rejected int `json:"rejected"`
	Repeats  int `json:"repeats"`

	AvgOffered  float64 `json:"avg_offered"`
	LastSeconds float64 `json:"last_seconds"`
	AvgSeconds  float64 `json:"avg_seconds"`

	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	Transport        int `json:"transport"`
	Fallbacks        int `json:"fallbacks"`

	Intent    string `json:"intent"`
	IntentAge int    `json:"intent_age"`

	Choices []ChoiceCount `json:"choices"`
}

// ChoiceCount is one objective and how many times the model has chosen it
// this run. The full sentence, not the kind: "go to pallet town" chosen six
// times is the finding, and "go-to chosen six times" hides it.
type ChoiceCount struct {
	Objective string `json:"objective"`
	Count     int    `json:"count"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./farm && go build ./...`
Expected: PASS.

**Step 5: Commit**

```bash
git add farm/spec.go farm/spec_test.go
git commit -m "farm: carry the llm planner tally on heartbeats"
```

---

### Task 2: Runner — push the tally onto the heartbeat snap

**Files:**
- Modify: `cmd/pokepilot/stats.go` (alias the types to farm, add snap to `statsPlanner`, push in `record`)
- Modify: `cmd/pokepilot/farm.go` (`storeStats`; `storeStatus` preserves Stats; `runFarmLLM` passes the snap)
- Modify: `cmd/pokepilot/main.go` (`runLLM` passes nil)
- Test: `cmd/pokepilot/stats_test.go`

**Step 1: Write the failing test**

In `cmd/pokepilot/stats_test.go`, add `"github.com/maestroi/pokepilot/farm"` to the imports, update the existing call site in `TestStatsPlannerTally` from

```go
	s := newStatsPlanner(&agent.LLMPlanner{}, func(any) { pushed++ })
```

to

```go
	s := newStatsPlanner(&agent.LLMPlanner{}, func(any) { pushed++ }, nil)
```

and append:

```go
// TestStatsPlannerPushesToSnap: the same tally the watch page renders is
// what the heartbeat carries, so the console and the runner's own page show
// one number. A sample tick between asks must not blank it, and a new lease
// must clear it.
func TestStatsPlannerPushesToSnap(t *testing.T) {
	snap := &heartbeatSnap{}
	snap.store(farm.Heartbeat{RunID: "r1"})
	s := newStatsPlanner(&agent.LLMPlanner{}, nil, snap)

	pallet := agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}
	obs := agent.Observation{Round: 2, RoundsLeft: 30}
	s.record(obs, 6, pallet, nil, time.Second)
	s.record(obs, 6, pallet, nil, time.Second)

	hb := snap.load()
	if hb.Stats == nil {
		t.Fatal("heartbeat carries no stats after two asks")
	}
	if hb.Stats.Round != 2 || hb.Stats.Rounds != 2 || hb.Stats.Repeats != 1 {
		t.Fatalf("stats = %+v, want round 2, rounds 2, repeats 1", hb.Stats)
	}
	if len(hb.Stats.Choices) != 1 || hb.Stats.Choices[0].Count != 2 {
		t.Fatalf("choices = %+v, want %q x2", hb.Stats.Choices, pallet)
	}

	// A sample tick between asks must not blank the tally.
	snap.storeStatus(farm.Heartbeat{RunID: "r1", Frame: 9})
	if hb = snap.load(); hb.Stats == nil || hb.Stats.Rounds != 2 {
		t.Fatalf("status tick blanked the stats: %+v", hb.Stats)
	}

	// A new lease clears it.
	snap.store(farm.Heartbeat{RunID: "r2"})
	if hb = snap.load(); hb.Stats != nil {
		t.Fatalf("new lease kept the old run's stats: %+v", hb.Stats)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/pokepilot -run 'TestStatsPlanner'`
Expected: FAIL — `not enough arguments in call to newStatsPlanner` and `undefined: (storeStats)`.

**Step 3: Write minimal implementation**

`cmd/pokepilot/stats.go`:

- Add `"github.com/maestroi/pokepilot/farm"` to the imports.
- Replace the `runStats` and `choiceCount` struct definitions with aliases (keep the long comment above `runStats`, appending one line):

```go
// The tally's wire type lives in farm: it rides the heartbeat to the wall,
// so the console and the watch page render one definition. The aliases keep
// this file and its tests on the old names.
type (
	runStats    = farm.LLMStats
	choiceCount = farm.ChoiceCount
)
```

- Add a `snap *heartbeatSnap` field to the `statsPlanner` struct.
- Change the constructor:

```go
func newStatsPlanner(inner *agent.LLMPlanner, push func(any), snap *heartbeatSnap) *statsPlanner {
	return &statsPlanner{inner: inner, push: push, snap: snap, counts: map[string]int{}}
}
```

- In `record()`, just before the final `if s.push != nil` block, add:

```go
	if s.snap != nil {
		s.snap.storeStats(s.stats)
	}
```

`cmd/pokepilot/farm.go`:

- In `storeStatus`, after `hb.Decision = s.hb.Decision` add:

```go
	hb.Stats = s.hb.Stats
```

and extend its doc comment's last line to mention the plan *and the stats*.

- Add after `storePlan`:

```go
// storeStats writes the latest planner tally. The copy is taken here —
// value plus a fresh Choices slice — so the snap never aliases the live
// tally that record() keeps mutating on the stepping goroutine.
func (s *heartbeatSnap) storeStats(st farm.LLMStats) {
	st.Choices = append([]farm.ChoiceCount(nil), st.Choices...)
	s.mu.Lock()
	s.hb.Stats = &st
	s.mu.Unlock()
}
```

- In `runFarmLLM`, change:

```go
	stats := newStatsPlanner(planner, m.TraceStats)
```

to

```go
	stats := newStatsPlanner(planner, m.TraceStats, snap)
```

`cmd/pokepilot/main.go`, in `runLLM`, change:

```go
	stats := newStatsPlanner(planner, m.TraceStats)
```

to

```go
	stats := newStatsPlanner(planner, m.TraceStats, nil)
```

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/pokepilot && go build ./...`
Expected: PASS. (The watch page is untouched: `m.TraceStats` still receives the same JSON keys.)

**Step 5: Commit**

```bash
git add cmd/pokepilot/stats.go cmd/pokepilot/stats_test.go cmd/pokepilot/farm.go cmd/pokepilot/main.go
git commit -m "pokepilot: ride the llm tally on the farm heartbeat"
```

---

### Task 3: Wall — store, persist and serve the tally

**Files:**
- Modify: `cmd/pokewall/wall.go` (Tile, tileRow, persistedTile, handleHeartbeat, snapshot, marshalStateLocked, loadState, settleRun)
- Test: Create `cmd/pokewall/llmstats_test.go`

**Step 1: Write the failing test**

Create `cmd/pokewall/llmstats_test.go`:

```go
package main

// The console's Play panel is the wall's data: heartbeats carry the tally,
// the dashboard serves it, a retry starts a fresh one, a finished run keeps
// its final one, and a restarted wall still has it.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func sampleStats(round int) *farm.LLMStats {
	return &farm.LLMStats{
		Round: round, RoundsLeft: 32 - round, Calls: round, Rounds: round,
		Repeats: 1, AvgOffered: 5.5, LastSeconds: 4.4, AvgSeconds: 3.1,
		PromptTokens: 947, CompletionTokens: 36,
		Intent: "get a move on the badge", IntentAge: 2,
		Choices: []farm.ChoiceCount{{Objective: "go to pallet town", Count: round}},
	}
}

func getDashboard(t *testing.T, url string) dashboardView {
	t.Helper()
	res, err := http.Get(url + "/v1/dashboard")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	defer res.Body.Close()
	var dash dashboardView
	if err := json.NewDecoder(res.Body).Decode(&dash); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	return dash
}

func TestDashboardCarriesLLMStats(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "llm1", Planner: "llm", Goal: "Earn the Boulder Badge."})

	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease = %v, %v; want a spec", spec, err)
	}

	// Before the first ask there is no tally, and the dashboard says so.
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 1}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	dash := getDashboard(t, srv.URL)
	if len(dash.Runs) != 1 || dash.Runs[0].Stats != nil {
		t.Fatalf("pre-first-ask stats = %+v, want nil", dash.Runs)
	}

	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 2, Stats: sampleStats(3)}); err != nil {
		t.Fatalf("heartbeat with stats: %v", err)
	}
	dash = getDashboard(t, srv.URL)
	s := dash.Runs[0].Stats
	if s == nil || s.Round != 3 || s.Repeats != 1 || len(s.Choices) != 1 || s.Choices[0].Objective != "go to pallet town" {
		t.Fatalf("dashboard stats = %+v, want round 3 with the choice tally", s)
	}
}

func TestWallResetsStatsOnRetryKeptOnDone(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "flaky-llm", Planner: "llm"})

	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease 1 = %v, %v; want a spec", spec, err)
	}
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 5, Stats: sampleStats(7)}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: spec.RunID, Attempt: spec.Attempt, Reason: "error", Detail: "wild battle"}); err != nil {
		t.Fatalf("finish 1: %v", err)
	}
	w.mu.Lock()
	reset := w.tiles["flaky-llm"].Stats == nil && w.tiles["flaky-llm"].Status == statusQueued
	w.mu.Unlock()
	if !reset {
		t.Fatal("retried run kept attempt 1's stats")
	}

	// Attempt 2 finishes cleanly: the final tally is the interesting number.
	spec, err = client.Lease(ctx)
	if err != nil || spec == nil || spec.Attempt != 2 {
		t.Fatalf("lease 2 = %v, %v; want attempt 2", spec, err)
	}
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 9, Stats: sampleStats(12)}); err != nil {
		t.Fatalf("heartbeat 2: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: spec.RunID, Attempt: spec.Attempt, Reason: "done"}); err != nil {
		t.Fatalf("finish 2: %v", err)
	}
	w.mu.Lock()
	t2 := w.tiles["flaky-llm"]
	kept := t2.Finished && t2.Status == statusDone && t2.Stats != nil && t2.Stats.Round == 12
	w.mu.Unlock()
	if !kept {
		t.Fatalf("finished run did not keep its final stats (stats=%+v)", t2.Stats)
	}
}

func TestWallStatePersistsLLMStats(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "wall-state.json")
	ctx := context.Background()

	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())
	enqueueViaHTTP(t, srv1.URL, farm.Spec{RunID: "persist-llm", Planner: "llm"})
	c1 := farm.NewClient(srv1.URL)
	spec, err := c1.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease = %v, %v; want a spec", spec, err)
	}
	if _, err := c1.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 3, Stats: sampleStats(5)}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	srv1.Close()

	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()

	dash := getDashboard(t, srv2.URL)
	if len(dash.Runs) != 1 || dash.Runs[0].Stats == nil || dash.Runs[0].Stats.Round != 5 {
		t.Fatalf("restored dashboard dropped the stats: %+v", dash.Runs)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/pokewall -run 'LLMStats|ResetsStats'`
Expected: FAIL — compile error, `unknown field Stats in struct literal` / `t.Stats undefined`.

**Step 3: Write minimal implementation**

In `cmd/pokewall/wall.go`:

- `Tile`: after the `StopSoFar string` field add:

```go
	// Stats is the llm planner's tally, last pushed by a heartbeat. Kept on
	// finish (the final tally explains the outcome), nilled on retry.
	Stats *farm.LLMStats
```

- `tileRow`: after `StopSoFar` add:

```go
	Stats      *farm.LLMStats `json:"stats,omitempty"`
```

- `persistedTile`: after `StopSoFar` add:

```go
	Stats       *farm.LLMStats `json:"stats,omitempty"`
```

- `marshalStateLocked`, in the `persistedTile{...}` literal, after `StopSoFar: t.StopSoFar,` add:

```go
			Stats:       t.Stats,
```

- `loadState`, in the `&Tile{...}` literal, after `StopSoFar: pt.StopSoFar,` add:

```go
			Stats:       pt.Stats,
```

- `handleHeartbeat`, after `t.StopSoFar = hb.StopSoFar` add:

```go
	t.Stats = hb.Stats
```

- `snapshot()`, in the `tileRow{...}` literal, after `StopSoFar: t.StopSoFar,` add:

```go
			Stats:      t.Stats,
```

- `settleRun`, in the retry branch after `t.Decision = ""` add:

```go
	t.Stats = nil
```

(The done path keeps Stats on purpose — do not touch it.)

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/pokewall && go build ./...`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/pokewall/wall.go cmd/pokewall/llmstats_test.go
git commit -m "pokewall: store, persist and serve the llm tally"
```

---

### Task 4: pokeui — card line + Play block

**Files:**
- Modify: `cmd/pokeui/ui/ui.js`
- Modify: `cmd/pokeui/ui/index.html` (CSS only)
- Test: `cmd/pokeui/console_test.go`

**Step 1: Write the failing test**

Append to `cmd/pokeui/console_test.go`:

```go
// TestUIRendersLLMStats: the console shows the same planner tally the
// runner's watch page renders — a line on each live llm card and a Play
// block in the detail pane — so a wandering run is visible without opening
// port 8099.
func TestUIRendersLLMStats(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		"statsLine", "playHTML",
		`r.stats`,
		`repeat picks`,
		`round ${s.round}`,
		`${s.avg_offered.toFixed(1)} avg`,
		`pbar`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	html := string(indexHTML)
	for _, want := range []string{`.pnums`, `.pchoice`, `.pwarn`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html missing %s", want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/pokeui -run TestUIRendersLLMStats`
Expected: FAIL — every `want` is missing.

**Step 3: Write minimal implementation**

`cmd/pokeui/ui/ui.js`:

a) After the `goalOf` helper, add:

```js
  function statsLine(r) {
    const s = r.stats;
    if (!s || r.planner === "scripted") return "";
    return `round ${s.round} (${s.rounds_left} left) · rep ${s.repeats}/${s.rounds} · think ${s.avg_seconds.toFixed(1)}s avg`;
  }
```

b) In `renderLive`, in the card template, after `<div class="stats"></div>` add:

```js
            <div class="llm-row" hidden>
              <div class="fact-k">llm</div>
              <div class="llm"></div>
            </div>
```

c) In `renderLive`, in the running branch (the `else` that sets `.pos` and `.stats`), after the `.stats` line add:

```js
        const llmRow = art.querySelector(".llm-row");
        const line = statsLine(r);
        llmRow.hidden = !line;
        if (line) art.querySelector(".llm").textContent = line;
```

d) Before `lastEventHTML`, add:

```js
  function playHTML(run) {
    const s = run.stats;
    if (!s || run.planner === "scripted") return "";
    const row = (k, v, warn) =>
      `<div class="prow"><span>${esc(k)}</span><span${warn ? ' class="pwarn"' : ""}>${esc(v)}</span></div>`;
    const nums =
      row("round", s.round + (s.rounds_left ? ` (${s.rounds_left} left)` : "")) +
      row("repeat picks", `${s.repeats} of ${s.rounds}`, s.rounds > 3 && s.repeats * 2 >= s.rounds) +
      row("think", `${s.last_seconds.toFixed(1)}s / ${s.avg_seconds.toFixed(1)}s avg`) +
      row("offered", `${s.avg_offered.toFixed(1)} avg`) +
      row("tokens", `${s.prompt_tokens} / ${s.completion_tokens}`) +
      row("rejected", String(s.rejected), s.rejected > 0) +
      row("transport", String(s.transport), s.transport > 0) +
      row("fallbacks", String(s.fallbacks), s.fallbacks > 0);
    const intent = s.intent ? `<p class="pintent">"${esc(s.intent)}" (${s.intent_age} rounds)</p>` : "";
    const top = (s.choices && s.choices[0]) ? s.choices[0].count : 1;
    const choices = (s.choices || []).map((c) =>
      `<div class="pchoice"><div class="pbar" style="width:${(100 * c.count) / top}%"></div>` +
      `<span>${esc(c.objective)}</span><span class="n">${c.count}</span></div>`).join("");
    return `<div class="block"><h3>Play</h3><div class="pnums">${nums}</div>${intent}<div class="pchoices">${choices}</div></div>`;
  }
```

e) In `renderDetail`, change:

```js
      + planHTML(run)
      + lastEventHTML(run);
```

to:

```js
      + planHTML(run)
      + playHTML(run)
      + lastEventHTML(run);
```

`cmd/pokeui/ui/index.html`, in the `<style>` block, after the `.plan-wait { ... }` rule add (the card's llm line needs no CSS: it inherits the bezel's ink color like the other fact values):

```css
  .pnums {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 4px var(--space-3);
  }
  .prow { display: flex; justify-content: space-between; gap: 8px; }
  .prow span:first-child { color: var(--bezel); }
  .prow .pwarn { color: var(--amber); }
  .pintent { margin: var(--space-2) 0 0; color: var(--lcd); white-space: pre-wrap; }
  .pchoices { margin-top: var(--space-2); }
  .pchoice {
    position: relative;
    padding: 2px 4px;
    margin-top: 1px;
    display: flex;
    justify-content: space-between;
    gap: 8px;
  }
  .pbar {
    position: absolute;
    left: 0; top: 0; bottom: 0;
    background: rgba(155, 188, 15, 0.16);
    z-index: 0;
    border-radius: 2px;
  }
  .pchoice span { position: relative; z-index: 1; }
  .pchoice .n { color: var(--lcd); }
```

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/pokeui && go build ./...`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/pokeui/ui/ui.js cmd/pokeui/ui/index.html cmd/pokeui/console_test.go
git commit -m "pokeui: show the llm tally on live cards and in the detail pane"
```

---

### Task 5: Full suite + end-to-end smoke

**Step 1: Run the whole suite**

Run: `go build ./... && go test ./...`
Expected: PASS everywhere. (If a failure is clearly inside files this plan does not touch — the tree carries unrelated in-flight work — name it and stop; do not adopt it.)

**Step 2: Smoke the data path through the real console proxy**

```bash
cd /home/maestro/Documents/projects/PokePilot
go run ./cmd/pokewall -http localhost:18080 -dumps /tmp/pw-dumps -state /tmp/pw-state.json &
go run ./cmd/pokeui -wall http://localhost:18080 -http localhost:18081 &
sleep 2

# llm run with a tally
curl -s -X POST localhost:18081/v1/specs -d '{"run_id":"smoke-llm","planner":"llm","goal":"Earn the Boulder Badge."}'
curl -s -X POST localhost:18081/v1/runs/smoke-llm/heartbeat \
  -d '{"run_id":"smoke-llm","frame":100,"stats":{"round":3,"rounds_left":29,"calls":4,"rounds":3,"repeats":1,"avg_offered":5.5,"last_seconds":4.4,"avg_seconds":3.1,"prompt_tokens":947,"completion_tokens":36,"intent":"get a move on the badge","intent_age":2,"choices":[{"objective":"go to pallet town","count":2}]}}'
curl -s localhost:18081/v1/dashboard | grep -o '"stats":{[^}]*"round":3[^}]*}'

# scripted run: no stats key at all
curl -s -X POST localhost:18081/v1/specs -d '{"run_id":"smoke-scr","planner":"scripted","starter":"squirtle","dest":"pallet"}'
curl -s -X POST localhost:18081/v1/runs/smoke-scr/heartbeat -d '{"run_id":"smoke-scr","frame":50}'
curl -s localhost:18081/v1/dashboard | grep -c '"stats"'   # want 1 (only smoke-llm)

kill %1 %2
```

Expected: the llm run's dashboard JSON carries the tally through pokeui's proxy; the scripted run has no `stats` key. Then open `http://localhost:18081` in a browser with the llm run still running (heartbeat every second from a real runner, or re-POST one) and confirm by eye: the card shows `round 3 (29 left) · rep 1/3 · think 3.1s avg`, and selecting the run shows the Play block with the amber "repeat picks" row and the choices bar.

**Step 3: Final commit check**

```bash
git status --short   # only the plan doc should be uncommitted; every task committed its own files
git log --oneline -5
```
