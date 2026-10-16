# RomPilot Admin — page redesign specifications

Uses the system in [DESIGN.md](DESIGN.md). Problems referenced as S1–S9 are in
[UI_AUDIT.md](UI_AUDIT.md). Two reference screens are fully specified: **Live**
(the workspace) and **Runs** (the data-heavy list + detail). The remaining pages
get layout direction that applies the same patterns. Their ownership boundaries are
fixed by `docs/superpowers/specs/2026-10-14-admin-ui-design.md` and are not
repeated here.

Capability rule: every control below maps to an existing API in
`web/src/shared/api/client.ts`. Where a control needs something that doesn't exist,
it's marked **[needs API]** and must not ship as a fake control.

---

## 1. Live — reference screen

### 1.1 Job

The operator picks a run and needs to answer, in this order: *What is it doing
now? Why? Is it healthy? What happened before? What can I do about it?* The page
is a single workspace for one selected run. Fleet administration supports it from
the edges.

### 1.2 Desktop layout (≥ 1440px)

```
┌ top bar ─────────────────────────────────────────────────────────────────────────────────┐
├──────────────┬───────────────────────────────────────────────────────────────────────────┤
│ RAIL 248     │ RUN HEADER  48px                                                          │
│              │ red-8f2a…c91d ⧉  ● Running  Pokémon Red · LLM · Nuzlocke   1×   [‖ Pause] [⋯]│
│ ▾ Active  3  ├────────────────────┬──────────────────────────┬───────────────────────────┤
│ ┌──┐ red-8f2a│ GAME        ● Live │ SEMANTIC MAP  Viridian…  │ GAME STATE  ₽3,420 · 2 ✦  │
│ │▒▒│ ● Run.. │                    │                          │ Charmander 18  41/52 ▬▬▬▭ │
│ └──┘ Route 2 │   160×144 frame    │   map, trail, player     │ Pidgey     11  30/30 ▬▬▬▬ │
│ ┌──┐ yell-3c │   pixel-perfect    │                          │ …6 slots                  │
│ │▒▒│ ‖ Paused│                    │                          │ Bag 14 · Dex 12/151       │
│ └──┘         │ [Emulator|Themed]  │ @ player ■ sprite · trail│ Milestones …              │
│ ▸ Recent  12 ├────────────────────┴──────────────────────────┴───────────────────────────┤
│              │ NOW BAND  72px                                                            │
│              │ Objective                │ Latest decision                    │ Frame    Round│
│              │ Beat Brock  ▬▬▬▭ 2/3     │ Heal at Pewter Center before gym…  │ 1,204,331  412│
│              │ Pewter City Gym          │ 14s ago · Why? →                   │ Pewter (12,8)  │
│              ├───────────────────────────────────────────────────────────────────────────┤
│              │ TIMELINE TRANSPORT  56px  (full width of workspace)                       │
│              │ [All ▾]  ◆──●──▲───◆──●●──✕───────────────────────▮ now   ◀ ▶  [Follow live]│
│──────────────├───────────────────────────────────────────────┬───────────────────────────┤
│ PROGRAMMING  │ RUN STORY                                     │ INSPECTOR                 │
│ Live: Gym …  │ 14:02 ◆ Objective: Beat Brock                 │ [Event][Planner][Checkpoints]│
│ Next: E4 sp… │ 14:05 ● Decision: Train on Route 3 (selected) │ [Artifacts][Run details]  │
│ [Skip][‖ Q]  │    ↳ question / decision / outcome            │  selected event evidence, │
│              │ 14:09 ▲ Recovery: stuck at ledge (2 tries)    │  actions beside it        │
└──────────────┴───────────────────────────────────────────────┴───────────────────────────┘
```

The first viewport (≈ 900px tall) contains: run header, triad, Now band and
timeline. Story and inspector begin at the fold. The page scrolls; the rail is
sticky with its own scroll.

### 1.3 Regions

**A. Rail (248px, sticky, `listbox`)**
- Groups: *Active* (running, paused, queued, leased; open by default) and *Recent*
  (ended, last 20; collapsed by default when Active has items). Group headers show counts.
- Row (64px): 64×58 frame thumbnail in a `--well` (grayscale + 55% opacity when
  ended; queued shows a `○ Queued` glyph instead of a frame), then three lines: run ID
  (mono, middle-truncated), `StatusPill` + at most one Tag (play style or "Debug
  coverage"), location · one stat (`text-meta`). Drop the recovery-profile and
  purpose badge pile (S7). Those facts go in Run details.
- Thumbnail `alt`: "red-8f2a at Route 2". The live state pulses on the pill glyph, not
  on a separate dot.
- Keyboard: ↑/↓ move, Enter selects, `j`/`k` select next/previous from anywhere
  on the page (not while typing). Selection writes `?run=`.
- **Programming footer** (moved from above the workspace): pinned to the rail
  bottom, collapsible. "Live now: <challenge>" / "Up next: <challenge> · Skip | Retry"
  and a `secondary sm` "Pause queue / Resume queue". Errors render inline. Same
  `programmingEntryAction`, `pauseProgramming`, `resumeProgramming` calls.

**B. Run header (48px)**
- Left: `CopyValue` run ID ("Copy run ID"), `StatusPill`, Tags: game, planner,
  play style. Then `text-meta` speed (`1×`). When paused, add the inline note
  "Safe to deploy and resume" (`--warn`).
- Right, by state. **Exactly one** filled action:

| Run state | Primary (filled) | Visible secondary | `⋯` menu |
|---|---|---|---|
| Running | Pause | — | Copy run ID · Visible on public site (checkbox) · Open public page · Open in Media · ─ · Cancel run… · Force quit worker… |
| Paused | Resume | — | same as running |
| Queued / leased | — | — | Copy run ID · Cancel run… |
| Ended, replay ready | Watch replay (plays in Game) | Open in Media | Copy run ID · Visible on public site · Open public page · ─ · Delete run… |
| Ended, no replay | Generate replay | — | as above |
| Ended, replay generating | — (inline `◐ Generating 42%` pill) | Open job in Media | as above |

- **Cancel run…** now opens `ConfirmDialog`: "Cancel run red-8f2a? The run stops
  and won't be retried. Recordings and checkpoints so far are kept." *(Confirm the
  last sentence against the backend before shipping; if it isn't guaranteed, drop it.)*
- **Force quit worker…** keeps its existing dialog copy.
- **Clone run** (`cloneRun`) is a visible secondary button in every state (operators
  reach for it often): on success, a toast with the new run ID and "Open" (selects it
  in the rail). It replaces the 1.8s label swap.
- Public visibility is a `menuitemcheckbox` showing current state. When the control
  is unavailable, the item is disabled with the reason as its description.
- Keyboard: `c` copies the run ID. `p` pauses/resumes, but only when a run is
  running/paused and focus isn't in an input. No shortcut ever triggers a
  destructive action.

**C. Triad (height `clamp(340px, 46vh, 760px)`)**
The row grows with the viewport. The Game column's width follows the row height at the
frame's 160:144 ratio, so the frame scales up instead of letterboxing; the map takes the
remaining width and Game state stays a fixed 300–340px column. Changes:
- Bay headers use `Bay` (36px, `text-title`). The Game header right side shows the state
  as a pill: `● Live`, `‖ Paused`, `■ Ended — last frame`, `▶ Replay 18:42 / 42:18`.
- The renderer control becomes a `SegmentedControl` (Emulator | Themed) on the
  frame's bottom-left, 28px, at 12px text, visible on hover/focus within the Game bay
  and always visible on touch. The theme `select` appears only in Themed mode, in a
  small popover off the segment, not as a permanent overlay. The theme notice moves into
  that popover.
- **Replay plays here.** For finished runs with a ready replay, Watch replay swaps the
  frame for the `replayVideoURL` `<video>` inside the same well, with native
  controls hidden and the transport (D) as the controller. Map and Game state then show the
  **final** semantic state, labelled "Final state — semantic history isn't
  available for replay moments" (PRODUCT.md honesty rule). If a timeline event with a
  persisted snapshot is selected, show that snapshot instead and label it
  "State at 14:05 (snapshot)". Remove the separate Playback `<video>` from the inspector.
- Map legend: 12px, uses the existing symbols, one line.
- Game state: party slots keep `hp/max` text and the HP meter, raised to 12px. The header
  readout `₽3,420 · 2 badges · Bag 14 · Dex 12/151` moves to `--fg-2` mono. Amber
  is reserved for warnings (S4).
- Tetris / Boxxle replacements keep their boards. Their key/value stats use `KeyValueList`.

**D. Now band (72px, 4 readouts in one fitted row)**
Replaces the 7-column status strip **and** the Settings / Current state / Plan /
Play fact wall (S7).

| Objective (1.4fr) | Latest decision (2fr) | Position (1fr) | Counters (auto) |
|---|---|---|---|
| Goal label (`text-strong`), goal summary (`text-meta`), `Meter` with `current / target` | Decision text, 2 lines, max 72ch. `text-meta`: age + "Why? →" opens Inspector › Planner | Location name + mono `(x,y)` | `text-readout` Frame and Round |

When there's no decision yet: "Waiting for the first plan" in `--fg-3`.

**E. Timeline transport (56px, full workspace width)**
Promoted from inside the inspector (S1-P1 in the critique). It uses the
existing `RunTimeline` data and `runTimelineFilters.ts`.
- Left: filter `select` ("All events", "Needs attention", "Recovery", "Failures",
  "LLM", "Skills", "Milestones", "Checkpoints", "System"). It replaces the 9 chips (≤ 4
  visible choices). Counts go in option labels.
- Track: markers keep their stable shapes (◆ objective, ● decision, ⚔ battle,
  ⚑ checkpoint, ▲ recovery, ✕ failure, ★ milestone). Each marker is a button with
  an `aria-label` ("14:05, decision: Train on Route 3"). Dense clusters collapse into a
  count marker that zooms on click.
- Playhead: at "now" for live runs. Dragging or clicking a marker selects that event and
  turns *Follow live* off. The *Follow live* toggle (`aria-pressed`) snaps back.
  For replays, the playhead is the video position and seeking moves the video.
- `[` / `]` step to the previous/next visible marker.
- Narrow: the track pans horizontally inside its own region. Filter and Follow live stay
  pinned.

**F. Run story (left, ~60%) + Inspector (right, ~40%, min 380px)**
- *Story*: the filtered timeline events, newest first (live and ended runs alike),
  so the latest event sits at the top while Follow live is on. Each entry: time (mono), glyph, one-line summary. The selected entry (synced
  with the timeline marker) expands in place to show question → decision → outcome
  as a 3-row `KeyValueList`. Raw exchange and trace tail sit behind a "Raw" disclosure.
- *Inspector* tabs (URL `?tab=`): Event, Planner, Media, Checkpoints, Artifacts, Details.
  **Media** holds replay generation/cancel, highlights and YouTube publishing, so
  every capability of the old inspector keeps a home.
  - **Event** (default): the selected event's evidence. A structured outcome uses
    `KeyValueList`. Related checkpoint → "Go to checkpoint". A failure event →
    "Investigate with AI" (`investigateTriage`, only when the event has a triage key)
    and "Open failure group".
  - **Planner**: today's Plan + Play + `DecisionTelemetry` content. The current
    question goes in a readable block (not `<pre>` at 11px), with the decision, intent and
    intent age, plus objective choice bars (`Meter`s with counts). Raw exchange stays behind
    a disclosure.
  - **Checkpoints** (`getRunCheckpoints`): a table with time, frame, location,
    "Paired agent state" (`✓ Available` / `▲ Missing`). Row → detail with
    `KeyValueList` of the known fields; JSON goes behind "Raw".
    **"Start a new run from here" is [needs API]**: the client has no
    checkpoint-start call and `RunSpec` has no checkpoint field. Until it exists, show
    no button. Add a one-line note: "Restarting from a checkpoint isn't available yet."
    When the API lands: a primary button on restartable rows, disabled with the
    backend reason on non-restartable rows.
  - **Artifacts** (`getRunArtifacts`): name, size, Open (`artifactContentURL`).
    The sha256 moves into a per-row `CopyValue` in a details disclosure, out of the primary list.
  - **Run details**: today's Settings rows + model/deployment/compute/seed/
    experiment as `KeyValueList`, plus the run debug bundle disclosure (`getRunDebug`).
- Action errors (`actionError`) render as an inline alert directly under the run
  header, not at the bottom of the page.

### 1.4 States

| State | Treatment |
|---|---|
| Loading first time | Skeleton of rail rows + triad wells + Now band readouts (shape-matched) |
| No runs at all | Workspace replaced by `EmptyState`: "No runs yet", action "New run" (primary) |
| No run selected but runs exist | Auto-select the newest active run (existing behaviour). If `?run=` doesn't exist: inline alert "Run red-xyz wasn't found. It may have been deleted." + select newest |
| Queued / leased | Game well shows `○ Queued` and the worker if leased; map/state show "Starts when a worker picks it up"; timeline empty state |
| Frame error / reconnecting | Keep the last frame; bottom-right well chip `▲ Last frame · reconnecting` |
| Wall stale | Shell fleet pulse shows `▲ Stale`; `ResourceState` stale alert above the workspace; data stays |
| Wall unreachable on first load | `ResourceState` error with "Retry now" |
| Paused | Pill `‖ Paused` + "Safe to deploy and resume"; primary = Resume |
| Ended, no replay / generating / failed | Header primary per table §1.3B; failed job → `✕ Replay failed` pill + "Retry" (`renderReplay`) |
| Tetris / Boxxle / non-Pokémon | Board replaces map; Game state shows game stats; party UI is never shown empty |
| Action in flight | Button busy label; other run actions stay usable |

### 1.5 Responsive

- **1024–1439**: rail 216px, or the 72px thumbnail strip below 1280 (see DESIGN §9).
  Inspector drops under Story (full width, tabs remain).
- **768–1023**: rail → left `Drawer` opened from a "Runs (3 active)" button
  in the run header. Triad → Game + Map side by side, Game state full width below.
- **< 768**: order = run header (primary action stays visible; others in `⋯`) →
  Game → Now band (stacked readouts) → Map → Game state → timeline (pans) →
  Story → Inspector tabs as an accordion. Programming footer moves into the rail drawer.

### 1.6 Acceptance (page level)

1. First viewport at 1440×900 shows run header, triad, Now band and timeline, with
   no scrolling.
2. Run ID, status, objective and latest decision each appear **once** above the fold.
3. Exactly one filled button in the workspace for each run state (per §1.3B).
4. Cancel run requires confirmation. No keyboard shortcut triggers a destructive action.
5. Finished replay plays in the Game well. No second `<video>` exists on the page.
6. Every timeline marker, rail row and checkpoint row is keyboard reachable with visible focus.
7. No text under 12px (`rg 'text-\[(7|8|9|10|11)px\]' web/src/operator/LiveView.vue` is empty).

---

## 2. Runs — reference data-heavy screen

### 2.1 Job

Find a finished run (by outcome, model, game, goal or experiment), understand it
from one row, open its evidence, and clean up history safely. Runs owns run facts.
Replay playback is a shortcut; replay management lives in Media (10-14 spec).

### 2.2 Desktop layout (≥ 1280px)

```
┌ Page header ─────────────────────────────────────────────────────────────────────────┐
│ Runs  1,284 finished                                   [Bulk cleanup…]  ⟳            │
├ Views ───────────────────────────────────────────────────────────────────────────────┤
│ [All runs] [Successful] [Fastest successful] [Has linked issue]                       │
├ FilterBar ───────────────────────────────────────────────────────────────────────────┤
│ [Go to run ID…]  [Game ▾] [Outcome ▾] [Model ▾] [Goal ▾]  More filters (2) ▾   1–50 of 312 │
│ Model: qwen3-32b ✕   Experiment: exp-12 ✕   Clear all                                │
├ DataTable ────────────────────────────────────────────────┬ DetailPane 440 ──────────┤
│ ☐ Outcome        Run                   Goal / progress   Model         Finished▼  Runtime  Replay   ⋯ │ red-8f2a…c91d ⧉  ✓ Goal complete ✕│
│ ☐ ✓ Goal done    red-8f2a…c91d         Beat Brock        qwen3-32b     12 min ago  1h 04m  ▶ Watch  ⋯ │ Pokémon Red · LLM · Charmander    │
│                  Pokémon Red · LLM     2 badges · Pewter vllm-a100-2                                 │ ───────────────────────────────── │
│ ☐ ✕ Stuck        yell-3c91…0a2e        Any%              gpt-oss-120b  1 h ago     3h 12m  Generate ⋯ │ Outcome   Goal complete           │
│                  Pokémon Yellow        0 badges · Rt 22   …                                          │ Detail    "Defeated Brock…"       │
│ …                                                                                    │ Progress / Model / Performance /  │
│                                                                                      │ Party / Milestones / Decisions    │
├──────────────────────────────────────────────────────────────────────────────────────┤ [Inspect in Live] [▶ Watch replay]│
│ 1–50 of 312                                            ‹ Previous   2 / 7   Next ›   │ [Open in Media]  ⋯ (Delete run…)  │
└──────────────────────────────────────────────────────────────────────────────────────┴───────────────────────────────────┘
```

### 2.3 Regions

**Page header.** "Runs" + total count (`total` from the archive API). Actions:
"Bulk cleanup…" (`secondary`) opens `RunCleanupPanel` inside a right `Drawer`, so
the destructive tool no longer sits above the data (S6). Refresh is an `IconButton`.

**Views row.** A `SegmentedControl`-styled row of saved presets that only set
existing filters/sort:
- *All runs*: no filters, sort finished desc.
- *Successful*: `successOnly`.
- *Fastest successful*: the existing `fastestSuccessful()` preset (success, sort frames asc).
- *Has linked issue*: **[needs API]** if the archive has no issue filter. Omit until it exists.

A view is "active" when the current filters equal its preset exactly. Otherwise no
view is highlighted.

**FilterBar.**
- *Go to run ID*: on Enter, if the value matches a run ID pattern, open that run's
  DetailPane via `getRun`. Not found → inline "No run with that ID". This
  is not free-text search; the archive API has none. Don't imply one.
- Visible facets (most used): Game, Outcome, Model, Goal. Under *More filters*:
  Deployment, Play style, Mode, Starter, Compute, Experiment. The count shows how many
  hidden facets are active.
- Facet option lists come from `facets.*` as today. Every filter value is in the URL
  (`#runs?game=pokemon-red&model=…&sort=frames&dir=asc&page=2&inspect=<id>`).
- Result range `1–50 of 312` right-aligned.

**DataTable (default columns).**

| Column | Content | Sort key |
|---|---|---|
| ☐ | Row selection | — |
| Outcome | `StatusPill` from `presentStatus('outcome', run)`: `✓ Goal complete`, `■ Stopped`, `✕ <reason>`; secondary: linked issue `#123` link if any | — |
| Run | Mono run ID (middle truncation, full in title) + `CopyValue` icon on row hover/focus; secondary: game Tag + planner (Scripted/LLM) Tag | — |
| Goal / progress | Goal text (truncate, title); secondary: `progressLabel` · badges count · location | `progress` |
| Model | Model name; secondary: deployment (mono). Scripted runs show "Scripted" in `--fg-3` | — |
| Finished | Relative time, absolute in `title` | `finished` (default ▼) |
| Runtime | `1h 04m` mono right-aligned; secondary: frames | `runtime` |
| Replay | `▶ Watch` (quiet link → `replayVideoURL`) / `◐ 42%` (link to Media › Jobs with that job selected) / `Generate` (quiet button, `renderReplay`) / `✕ Retry` | — |
| ⋯ | ActionMenu: Inspect in Live · Open in Media · Copy run ID · ─ · Delete run… | — |

The other sort keys (`rounds`, `planner_time`, `strategic_calls`, `frames`) are in a
"Sort by" menu at the right end of the header row, with the same URL params. Their values
show in the DetailPane. Today's 11-select grid and the separate direction select are
removed (S7).

Inline badge-as-filter buttons are replaced by Tags. Clicking a Tag sets that filter
(button with `aria-label="Filter by Pokémon Red"`).

**Row interaction.** Click / Enter opens the DetailPane and sets `?inspect=`. The
row stays highlighted. ↑/↓ moves the selection while the pane is open. `/` focuses
"Go to run ID". "Inspect in Live" navigates in-app (`#live?run=` via the router, no
full reload; S8).

**DetailPane (440px).** It replaces the inline "Details" expand.
- Header: `CopyValue` run ID, `StatusPill`, close.
- Sections (`KeyValueList`, each with a `text-title` heading):
  *Outcome* (result, detail text in a 72ch block, linked issue) ·
  *Progress* (goal, progress, location, badges, milestones) ·
  *Model* (model, deployment, compute, quantization, reasoning effort, decision engine,
  seed, experiment → link to Analytics › Experiments) ·
  *Performance* (rounds, frames, runtime, planner time, strategic calls, plans executed,
  steps skipped, rejected, prompt/completion tokens; all mono right-aligned) ·
  *Party* (compact list) ·
  *Decisions* (`DecisionTelemetry` without the feed, existing `:show-feed="false"`).
- Footer: primary "Inspect in Live", secondary replay action (as in the table), `⋯`
  with Open in Media, Copy run ID, Delete run….

**Selection and bulk.** Checking any row swaps the FilterBar for the bulk bar:
"3 selected · Delete… · Clear selection". The header checkbox selects the
visible page only, labelled as such. Delete… opens `ConfirmDialog`: "Delete 3 runs?
Their stored run data, recordings and replay videos are removed. This can't be
undone." It lists up to 5 IDs + "and N more". Above 10 runs, `requireText` asks for the count.

**Pagination.** Footer range + Previous / page `n / N` / Next, as today, with
`PAGE_SIZE` unchanged. Page is in the URL.

**Errors.** Replay action errors render inline in the affected row's Replay cell
(`✕ Couldn't queue · Retry`) and in the live region, not as a page-bottom paragraph.
Delete errors go to the dialog (it stays open with the message) or to a toast for bulk
partial failures ("Deleted 9 of 10 runs; red-xyz failed: …").

### 2.4 States

| State | Treatment |
|---|---|
| Loading | Table frame with header + 8 skeleton rows of real row height |
| Empty archive | `EmptyState` "No finished runs yet" + "New run" |
| Empty with filters | "No runs match these filters" + "Clear filters"; active chips stay visible |
| API error | `ResourceState` error inside the table frame + Retry; FilterBar stays usable |
| Stale | Stale alert above the table; rows stay |
| `?inspect=` unknown | DetailPane shows "Run not found. It may have been deleted." + Close |

### 2.5 Responsive

- **1024–1279**: DetailPane becomes an overlay sheet; the Model column hides (it's in
  the pane).
- **768–1023**: hide Runtime and Model; Views row scrolls horizontally.
- **< 768**: list rows:
  line 1 `StatusPill` · run ID · finished;
  line 2 goal · progress;
  line 3 model · runtime · replay action.
  Checkbox moves to a long-press-free "Select" mode toggled from the page header.
  The DetailPane becomes a full-screen sheet.

### 2.6 Acceptance (page level)

1. No more than 4 facet controls visible by default. All filters, sort, page and
   inspected run survive reload and back/forward.
2. Each row is two lines at most, in the columns above.
3. Delete is only reachable through a menu or the bulk bar, and always confirms with the
   count and the IDs.
4. Bulk cleanup isn't rendered on page load; it opens in a drawer.
5. Inspect in Live doesn't reload the document.
6. At 375px the page has no horizontal scroll and every action is reachable.

---

## 3. Other pages — direction (detailed specs follow when each task starts)

These pages apply the same patterns. Ownership and scope are fixed in the 10-14 spec
and tracked in #2672–#2676.

**Failures**: fully specified in §4 (T-40).

**Analytics (Overview + Experiments tabs).** The KPI tiles are replaced by a one-line
health strip of readouts ("Completed 1,284 · Objective wins 412 (32%) · ≥1 badge 61%").
Bays: Run outcomes (stacked bar, existing data), Badge distribution (bar), LLM
workload (table), Terminal outcomes (table). Experiments tab: list + detail. "Start
paired experiment" is the primary action. Endless and paired experiments are
labelled as distinct types. Charts follow the dataviz guidance: direct labels, and
colours from the semantic tokens only when they carry meaning.

**Media (Library, Studio, Broadcasts, Jobs)**: fully specified in §5 (T-43).

**Operations (Overview).** Follow the operations concept: health strip, Workers table
(worker → current run link to Live), Queue & leases, Recent operational activity.
Active attempts and Recent outcomes tables are removed (they're Live's and Runs'
data). LLM deployments → Settings › Models. Paired experiments → Analytics › Experiments.

**New run (Form page, `#new-run`).** The existing ToolsView form in a single
720px column, grouped into fieldsets (Game & goal, Planner & model, Run options),
advanced fields behind a disclosure, sticky footer with "Queue run" (primary). The
success receipt keeps the run ID `CopyValue` + "Open in Live".

**Settings (tabs: Models, Destinations, Renderer, About).** Models = `LLMDeploymentsPanel`
restyled. Destinations = broadcast destination config (secrets stay server-side).
Renderer = theme assets (from Media › Assets/Themes). About = wall version, UI build
link, connection details (moved from the top bar).

---

## 4. Failures — list + detail (T-40)

### 4.1 Job

Answer, in order: *Which failure groups are open and how bad are they? Is someone
(an issue, a solver) already on it? Which runs show it?* Then act: investigate
with AI, open the evidence in Live, dismiss noise, or clean up the matching run
history. Failures owns triage groups; run facts stay in Runs and Live.

Data: `getTriage()` returns every group (open and resolved) in one response with no
server paging, filtering or sort. The page therefore filters, sorts and pages the
loaded list client-side (50 per page, DESIGN §6). Polling stays at 5s.

### 4.2 Desktop layout (≥ 1280px)

```
┌ Page header ──────────────────────────────────────────────────────────────────────┐
│ Failures  1 open                              [Dismiss unlinked (3)…]  ⟳          │
├ Views ────────────────────────────────────────────────────────────────────────────┤
│ [Open 1] [Resolved 361] [All 362]                                                 │
├ FilterBar ────────────────────────────────────────────────────────────────────────┤
│ [Search pattern, key or run ID…]  [Issue ▾]        Sort by [Occurrences ▾]  1–1 of 1 │
│ Search: run-m0id…7k ✕   Clear all                                                    │
├ DataTable ───────────────────────────────────────────┬ DetailPane 440 ─────────────┤
│ ☐ Status        Failure                  Count  Latest run        Updated   ⋯ │ f452eab3b3f98efe ⧉ ✕ Regressed ✕│
│ ☐ ✕ Regressed   recover from terminal…      15  run-m0id…7k ⧉     3 h ago   ⋯ │ Failure  pattern (72ch)         │
│   #2597 ↗       f452eab3b3f98efe ⧉               2 runs                         │ Issue    #2597 · open · verif…  │
│                                                                               │ Solver   9 attempts, latest …   │
│                                                                               │ Example runs (full IDs ⧉ Open in Live)│
├───────────────────────────────────────────────────────────────────────────────┤ [Investigate with AI] [Open issue ↗]│
│ 1–1 of 1                                     ‹ Previous  1 / 1  Next ›         │ Dismiss…  Delete matching runs… │
└───────────────────────────────────────────────────────────────────────────────┴─────────────────────────────────┘
```

### 4.3 Regions

**Page header.** "Failures" + the open-group count. Actions: "Dismiss unlinked (n)…"
(`secondary`, only when n > 0; confirms; `dismissTriages`) and a Refresh `IconButton`.
No description paragraph, no eyebrow.

**Views row.** Segmented presets over `state`: *Open* (default), *Resolved*, *All*,
each with its count. "Resolved" follows `isResolvedGroup()` (aligned with the wall's
actionable rule). It replaces the "Show resolved" toggle and the second panel.

**FilterBar.**
- *Search* (240px, `/` focuses it): substring match on group key, pattern, detail,
  fingerprint and example run IDs. It's client-side over the loaded list, so it is a
  real search here (unlike Runs).
- Facet: *Issue* (Any / Has issue / No issue).
- *Game* facet: **[needs API]**. Triage groups carry no game field. Not shipped.
- Live's "Open in Failures" (failure events) links to `#failures?state=all&q=<run id>`,
  so the search finds that run's groups. `?run=` stays Live's selection and is not a
  Failures key, so the nav link from Live never filters this page by accident.
- Sort select: Occurrences (default, desc) or Last updated (issue `updated_at`, desc).
- Result range right-aligned. Chips + "Clear all" beneath, as in Runs.
- URL: `?state=resolved&issue=none&q=…&sort=updated&page=2&failure=<key>#failures`.
  Defaults are omitted. Every change resets `page` to 1.

**DataTable.**

| Column | Content |
|---|---|
| ☐ | Row selection (bulk bar) |
| Status | `StatusPill` from `presentFailureGroup()`: `✓ Resolved`, `✕ Regressed` (verification regressed), `▲ Quarantined` (circuit open), `▲ Needs issue` (no issue), `✕ Open`; secondary: issue `#n ↗` |
| Failure | Pattern (truncate, full in `title`); secondary: group key mono + copy on hover/focus |
| Count | Occurrences, mono right-aligned |
| Latest run | Middle-truncated run ID (full in `title`) + copy; secondary: "n runs" |
| Updated | Issue `updated_at`, relative, absolute in `title`; "—" without an issue |
| ⋯ | ActionMenu: Open latest run in Live · Copy group key · ─ · Dismiss… (unlinked only) · Delete matching runs… |

First seen / last seen: **[needs API]** (no per-group timestamps besides the issue's
`updated_at`). Not shipped.

**Row interaction.** Click / Enter opens the DetailPane and sets `?failure=<key>`.
↑/↓ moves the selection while the pane is open; Esc closes it and returns focus to the row.

**DetailPane (440px; overlay sheet < 1280; full screen < 768).**
- Header: group key (mono) + "Copy group key", `StatusPill`, close.
- *Failure*: pattern as a 72ch prose block; the raw example message in a "Details"
  disclosure; occurrences; fingerprint as a copyable mono value.
- *Issue* (`KeyValueList`): `#n ↗`, status / resolution, verification state (+ required
  runs), quarantine count. *Solver*: latest attempt summary; every attempt (model, state,
  note) behind a disclosure.
- *Example runs*: **every** example ID in full (mono, wraps, never cut at 18 chars),
  each with "Copy run ID" and "Open in Live". Open in Live navigates in-app to
  `#live?run=<id>&failure=<key>`.
- Footer: primary **Investigate with AI** (`investigateTriage`) when the group has an
  issue and isn't resolved. Otherwise it's disabled with the reason as adjacent
  `text-meta`: "Needs a linked issue first" or "Already resolved". Creating an issue
  from here is **[needs API]**; "Open issue" only links an existing issue. Secondary:
  "Open issue ↗". Then "Dismiss…" (unlinked dismissable groups only) and
  "Delete matching runs…" (`danger`).

**Live strip.** When Live's URL has `failure=<key>`, a one-line strip sits under the run
header: "From failure group `<key>`" + "← Back to failure group" (in-app link to
`#failures?failure=<key>`, keeping the Failures filters that travelled in the URL) +
a close button that drops the param. Choosing another run in the rail drops it too.

**Selection and bulk.** Checking rows swaps the FilterBar for the bulk bar:
"n selected · Delete matching runs… · Dismiss… · Clear selection". Dismiss is disabled
with "Only unlinked groups can be dismissed" when no selected group qualifies, and
otherwise dismisses just the qualifying ones.

**Confirmations.**
- Delete matching runs: first counts the finished runs that still match
  (`getDashboard({status:'done'})` + `matchingRunsForGroups`). Zero → toast, no dialog.
  Dialog: "Delete n runs for issue #2597?" naming the pattern, S3 artifacts and replay
  cache, "This can't be undone." Above 10 runs, `requireText` asks for the count.
  Progress replaces the message while deleting.
- Dismiss: "Dismiss n unlinked failure groups?" Run data stays; future occurrences
  reappear. Groups that gained an issue meanwhile are reported as skipped.

**Receipts and errors.** Copy, investigate, dismiss and cleanup results go to the
page's polite live region (toast). Action errors render as an inline alert under the
page header; investigate errors also appear in the pane footer.

### 4.4 States

| State | Treatment |
|---|---|
| Loading | Table frame + 8 skeleton rows |
| No groups at all | `EmptyState` "No failure groups" + "New actionable failure groups appear here." |
| Empty with filters / view | "No failure groups match" + "Clear filters"; chips stay |
| API error | `ResourceState` error inside the table frame + Retry |
| Stale | Stale alert above the table; rows stay |
| `?failure=` unknown | Pane: "Failure group not found. It may have been resolved or removed." + Close |

### 4.5 Responsive

- **1024–1279**: pane becomes an overlay sheet; Updated column hides.
- **768–1023**: Latest run hides (it's in the pane).
- **< 768**: list rows: line 1 `StatusPill` · count · updated; line 2 pattern (opens the
  pane); line 3 key · issue. A "Select" toggle in the page header enables checkboxes.
  The pane is a full-screen sheet.

### 4.6 Acceptance (page level)

1. Example run IDs in the pane are full strings with copy; nothing is cut at 18 chars.
2. Open in Live carries `failure=<key>`; Live shows the "From failure group" strip with
   a working back link that restores the selected group.
3. Investigate with AI is disabled with a visible reason when there's no issue (or it's resolved).
4. View, filters, sort, page and the inspected group survive reload and back/forward.
5. Dismiss and Delete matching runs always confirm; delete states the count.
6. At 375px the page has no horizontal scroll and every action is reachable.
7. No text under 12px; no Tailwind palette colours, `Panel` or `StatusBadge` in `FailuresView`.

---

## 5. Media — Library, Studio, Broadcasts, Jobs (T-43)

### 5.1 Job

Answer, in order: *What media exists for which run, and can I watch it? Is any of it
public, and is anything on air right now? What is being generated, and did it fail?*
Then act: play, publish or hide, generate a derived output, start or stop a broadcast,
retry or cancel a render job. Media owns outputs and their publication; run facts stay
in Runs and Live, destinations and renderer themes in Settings.

Data (existing calls only):
- Library: `getDashboard({active})` + `getDashboard({status:'done', limit:48})` (+ `getRun`
  for a linked run outside that page), `getSpectatorControl`, `getMediaRenderJobs(100)`
  for recording state; per selected run `getReplayStatus`, `getHighlightStatus`,
  `getYouTubePublishStatus` (shared `useRunMedia`, also used by Live's inspector).
- Studio: finished runs + `getHighlightStatus` / `renderHighlights`.
- Broadcasts and the on-air strip: running runs from `getDashboard({active})` and
  `getBroadcastStatus` per running run (3s), `getBroadcastControl` (30s).
- Jobs: `getMediaRenderJobs(50)` (2s), `retry/cancel/deleteMediaRenderJob`.

Library rows are **source runs**, one per run: its full replay and highlights are outputs
of that run, not separate items (10-14 spec). A per-output list (format filter, Shorts,
episodes) is **[needs API]**: there is no output index, only per-run status calls.

### 5.2 Desktop layout (≥ 1280px, Library with a selection)

```
┌ Page header ───────────────────────────────────────────────────────────────────────┐
│ Media                                                          [↗ Open public site] │
├ Tabs ──────────────────────────────────────────────────────────────────────────────┤
│ Library   Studio   Broadcasts   Jobs                                                │
├ On-air strip (only while a broadcast is live) ─────────────────────────────────────┤
│ On air  ● Live  Twitch  Source run-1nrt…rms4 · Game  32m 18s  [Open source run] [Manage broadcast] │
├ Views ─────────────────────────────────────────────────────────────────────────────┤
│ [All 46] [Finished 40] [Live 6]                                                  ⟳ │
├ FilterBar ─────────────────────────────────────────┬ DetailPane 440 ───────────────┤
│ [Search run ID, game or goal…] [Replay ▾] [Public ▾]  46 of 46 │ run-10c9…ko0h ⧉  ✓ Replay available ✕ │
├ DataTable ─────────────────────────────────────────┤ ▶ in-pane player (video)      │
│ Replay              Source run          Public  When  ⋯ │ [Full replay|Highlights]      │
│ ● Running           run-1nrt…rms4 ⧉     Public  37m   ⋯ │ Full replay ✓  Generate/Cancel │
│ ✓ Replay available  run-10c9…ko0h ⧉     Public  37m   ⋯ │ Highlights  ○  Create in Studio│
│ ◐ Rendering · 43%   run-3oy7…24do ⧉     Private 4 h   ⋯ │ Public site [Replay public] │
│ …                                                       │ YouTube (when configured)      │
├─────────────────────────────────────────────────────┤ Source run (KeyValueList)     │
│ Live runs and the 48 most recent finished runs   Browse all source runs in Runs │ [Open replay ↗] [Open source run] │
└─────────────────────────────────────────────────────┴───────────────────────────────┘
```

### 5.3 Regions

**Page header.** "Media" + "Open public site" (`secondary`). No description, no eyebrow,
no per-tab intro cards.

**Tabs.** Underline tabs Library · Studio · Broadcasts · Jobs as links (`aria-current`).
Switching tabs keeps the source run (`run`) and drops the job (`job` only means Jobs).

**On-air strip.** Rendered across every tab only while the replay host reports a
broadcast as `live`/`healthy` (`isBroadcastOnAir`). One line per live broadcast:
`● Live` pill, destination label, source run (middle-truncated, full in `title`) + game,
elapsed from `started_at_unix_ms`. Actions: "Open source run" (Live) and "Manage
broadcast" (Broadcasts with that run selected). `starting`, `reconnecting`, a running
game or public visibility never show the strip. External channel links are
**[needs API]** (destinations carry no channel URL).

**Library.**
- Views: All / Finished / Live with counts.
- FilterBar: search (client-side over run ID, game, goal; `/` focuses it), *Replay*
  facet (Ready / Generating / Failed / No replay, from `recordingSummary`), *Public site*
  facet (Public / Private). Chips + "Clear all". Format facet: **[needs API]**.
- DataTable: Replay (`StatusPill`: run status for live runs, recording state for finished
  runs via `presentRecording`; secondary "Full replay" / "Live run · replay after it ends"),
  Source run (middle-truncated + copy; secondary game · goal), Public site (`Tag` +
  "★ Featured"), When (relative, absolute in `title`), ⋯ `ActionMenu`: Open source run ·
  Copy run ID · Open render job · ─ Open replay/live ↗ · Visible on public site
  (`menuitemcheckbox`) · Feature live / Clear feature.
- Footer: scope note + "Browse all source runs in Runs".
- DetailPane (`?run=`): header run ID + copy + recording pill + close. In-pane `<video>`
  player for the replay, with a Full replay / Highlights segmented control when the reel
  is ready; when there is no video, the well states why (`recordingSummary.detail`, or
  "This run is still live"). Sections: *Full replay* (state, Generate/Retry replay,
  Cancel generation… confirm, Open video file, Open render job), *Highlights* (state,
  Create/Open in Studio, Manifest), *Public site* (visibility toggle with the existing
  "Publish on start / Public live / Replay public / Publish replay" labels, Feature live),
  *YouTube* (when configured and the replay is ready; upload confirm), *Source run*
  (`KeyValueList`). Footer: primary "Open replay ↗" when public, "Open source run".

**Studio.** Source recording list (finished runs, `?run=`) + "Highlight reel" bay:
state, moments and length, Generate/Retry highlights, in-bay player when ready,
Manifest, Open render job, Open in Library. Shorts, episodes, edit plans and
publication review are **[needs API]** (#796, #797–#803) and are not shown, not even
as disabled buttons.

**Broadcasts.** "Running runs" listbox: each running run with its actual broadcast
`StatusPill` (`presentBroadcast`: Offline ○, Starting ◐, Live ●, Reconnecting ◐,
Stopping ◐, Failed ✕), destination and elapsed. Selecting sets `?run=`. "Output" bay
for the selected run: destination select (configured state, host, "Destination settings →"
to Settings › Destinations), Encoding disclosure (size, fps, bitrate, preset), primary
**Start broadcast** (disabled with the reason as `text-meta` when no destination is
configured), **Stop broadcast** (`danger`), restart note, and status `KeyValueList`
(destination, output, elapsed, reconnects, source run) + last error. Credentials stay
on the replay host.

**Jobs.** Views: All / Active / Queued / Failed / Ready / Cancelled with counts over the
loaded jobs. A `?run=` filter shows as a removable chip. DataTable: Status
(`presentJob`, failure class + retries as secondary), Source run (middle-truncated +
copy; job ID secondary), Progress (`Meter` + segments, size), Renderer, Updated, ⋯
(Open source run · Copy job ID · Copy run ID · ─ Retry · Cancel render… · Remove job
record…). DetailPane (`?job=`): progress, failure alert with the raw error in a
"Details" disclosure, job `KeyValueList` (source run, stage, renderer + explanation,
worker/lease, retries, output size, runtime, created, updated, artifact key). Footer:
Retry (primary, when retryable) · Open source run · Cancel render… · Remove job record….

**Confirmations (existing copy).** Cancel render: "Cancel replay rendering for <run>? The
current render attempt will stop and can be retried later." Remove: "Remove render job
for <run>?" + the ready/terminal detail. Cancel generation and YouTube upload keep Live's
copy. Retry and broadcast start/stop act immediately, as before.

**Routing.** `#media/<tab>?run=…&job=…&media_q=…&media_view=…&recording=…&visibility=…&job_state=…`.
Defaults are omitted; Media-only keys are prefixed so Failures' `q`/`state` never filter
Media. Legacy links resolve and are rewritten in place: `#media/replays` → Library,
`#media/render-jobs` → Jobs, `#media/themes` and `#media/assets` → Settings › Renderer,
and any Library link carrying `job=` (`#media?job=…`) → Jobs with that job selected.

### 5.4 States

| State | Treatment |
|---|---|
| Loading | Table frame + skeleton rows; pane skeleton with a 16:9 well |
| Empty library / jobs | `EmptyState` "No media yet" / "No replay render jobs yet" |
| Empty with filters | "No media matches" / "No render jobs match" + Clear filters / Show all jobs |
| API error | `ResourceState` error inside the frame + Retry |
| Stale | Stale alert above the table; rows stay |
| `?run=` unknown | Warning with "Inspect source run"; pane "Run not found" |
| `?job=` not in the recent list | Pane: "No matching render job is in the recent job list for this selection." + Show all jobs |
| Broadcast status unavailable | Neutral "■ Status unavailable" pill per run; warning in the Output bay |
| No running run | Broadcasts empty state with a link to Settings › Destinations |

### 5.5 Responsive

- **1024–1279**: panes become overlay sheets; Renderer and When hide while a pane is open.
- **768–1023**: Progress and Public site columns hide (they're in the pane).
- **< 768**: three-line lists (pill · meta / run ID / game · goal), `lg` controls, full-screen panes,
  Broadcasts bays stack.

### 5.6 Acceptance (page level)

1. Tab and selected item (`run` / `job`) survive reload and back/forward (`test/mediaQuery.test.ts`).
2. The on-air strip renders only for a backend-reported live broadcast.
3. Legacy media links (old sub-tabs, `?job=`) resolve to Jobs with the job selected (`test/mediaNavigation.test.ts`).
4. No per-tab intro cards; Studio shows only implemented capabilities.
5. Replay generate/cancel, highlights, YouTube publishing, public visibility/feature, broadcasts
   and render job retry/cancel/remove all remain, with their confirms.
6. At 375px no horizontal scroll; every action is reachable.
7. No text under 12px, palette colours, `Panel`, `StatusBadge` or game-specific strings in Media files.

