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
| Running | Pause | — | Copy run ID · Run again with same settings · Visible on public site (checkbox) · Open public page · Open in Media · ─ · Cancel run… · Force quit worker… |
| Paused | Resume | — | same as running |
| Queued / leased | — | — | Copy run ID · Cancel run… |
| Ended, replay ready | Watch replay (plays in Game) | Open in Media | Copy run ID · Run again… · Visible on public site · Open public page · ─ · Delete run… |
| Ended, no replay | Generate replay | — | as above |
| Ended, replay generating | — (inline `◐ Generating 42%` pill) | Open job in Media | as above |

- **Cancel run…** now opens `ConfirmDialog`: "Cancel run red-8f2a? The run stops
  and won't be retried. Recordings and checkpoints so far are kept." *(Confirm the
  last sentence against the backend before shipping; if it isn't guaranteed, drop it.)*
- **Force quit worker…** keeps its existing dialog copy.
- **Run again with same settings** (`cloneRun`): on success, a toast with the new
  run ID and "Open" (selects it in the rail). It replaces the 1.8s label swap.
- Public visibility is a `menuitemcheckbox` showing current state. When the control
  is unavailable, the item is disabled with the reason as its description.
- Keyboard: `c` copies the run ID. `p` pauses/resumes, but only when a run is
  running/paused and focus isn't in an input. No shortcut ever triggers a
  destructive action.

**C. Triad (height `clamp(320px, 40vh, 420px)`)**
Structure and proportions stay as they are (1 : 1.2 : 0.55). Changes:
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
- *Story*: a chronological list of the filtered timeline events, newest at the bottom
  for live runs with auto-scroll while Follow live is on, and oldest-first for ended
  runs. Each entry: time (mono), glyph, one-line summary. The selected entry (synced
  with the timeline marker) expands in place to show question → decision → outcome
  as a 3-row `KeyValueList`. Raw exchange and trace tail sit behind a "Raw" disclosure.
- *Inspector* tabs (URL `?tab=`):
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

**Failures (List + detail).** FilterBar (state: open / resolved, game, has issue).
Table: group key (summary), count, latest run, first/last seen, linked issue. DetailPane:
pattern description, example runs as full `CopyValue` IDs (no 18-char truncation)
with "Open in Live" (carries `?failure=<key>` so Live shows a "From failure
group <key>" strip with a back link), and actions: "Investigate with AI"
(primary when an issue exists; otherwise disabled with reason "Open an issue
first" + "Open issue"), Dismiss…, Delete matching runs… (both confirm).

**Analytics (Overview + Experiments tabs).** The KPI tiles are replaced by a one-line
health strip of readouts ("Completed 1,284 · Objective wins 412 (32%) · ≥1 badge 61%").
Bays: Run outcomes (stacked bar, existing data), Badge distribution (bar), LLM
workload (table), Terminal outcomes (table). Experiments tab: list + detail. "Start
paired experiment" is the primary action. Endless and paired experiments are
labelled as distinct types. Charts follow the dataviz guidance: direct labels, and
colours from the semantic tokens only when they carry meaning.

**Media (Library, Studio, Broadcasts, Jobs).** Follow the media-workspace concept
image, restyled to this system: Library is list + detail with an in-pane player;
an on-air strip appears only when a broadcast is actually live (glyph states per DESIGN §4.3).
Remove the per-tab intro cards. Studio shows only capabilities that exist.

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
