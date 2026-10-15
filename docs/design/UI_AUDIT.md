# RomPilot Admin — UI audit

Status: audit for the 2026-10 redesign. Companion documents:
[DESIGN.md](DESIGN.md) (system), [PAGE_REDESIGN.md](PAGE_REDESIGN.md) (page specs),
[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) (tasks).

## Evidence and method

- **Screenshots.** The brief points to `docs/ui-before/`; that directory does not
  exist in this branch, and `admin.rompilot.app` is behind Cloudflare Access, so no
  fresh captures were taken. Visual evidence comes from the committed concepts in
  `docs/superpowers/specs/assets/2026-10-14-admin-ui/` and from the templates.
  **Action:** add the eight captures to `docs/ui-before/` before task T-01 (see the
  plan) so before/after comparison has a baseline.
- **Prior decisions honoured.** `PRODUCT.md`, root `DESIGN.md`, the agreed IA in
  `docs/superpowers/specs/2026-10-14-admin-ui-design.md` (issues #2670–#2676), and
  the 2026-10-14 critique (`.impeccable/critique/…app-vue.md`, score 23/40).
  This audit builds on them; it doesn't argue with them again.
- **Code reviewed.** Shell (`App.vue`, `AppShell.vue`, `base.css`), shared
  primitives (`Panel`, `StatusBadge`, `ConfirmDialog`, `ResourceState`), the full
  `LiveView` and `RunArchiveView` templates, and section structure of every other
  operator view.

### Measured facts (from `web/src/operator` + `web/src/shared/components`)

| Measure | Value | Why it matters |
|---|---|---|
| `:focus-visible` rules in the operator UI | **0** | Tailwind preflight removes outlines, so keyboard users can't see focus anywhere |
| Text set at 9px or smaller (`text-[9px]`, `[8px]`, `[7px]`) | **87** | Below a readable floor for an all-day console |
| Text at 10px | **257** | 10px is the de facto body size of the console |
| Uses of off-system Tailwind palette (`slate-*`, `cyan-4/5xx`, `rose-*`, `rounded-md/lg`) | **~600 across 20 files** | A second visual system next to `--poke-*` |
| Accent colours meaning "action" | 3 (`--poke-cyan`, `cyan-500`, `violet-300` for replay) | Same meaning, three colours |
| Primary nav destinations | 7 peers (Live, Runs, Failures, Analytics, Media, Operations, Tools) | "Tools" is one form; agreed IA removes it |
| Live action cluster | up to 7 equal 10px buttons + 2 header actions | No hierarchy; Cancel is unguarded beside Pause |
| Runs filter controls visible at once | 11 selects + checkbox + 2 sort selects | All visible before the first row |

## Systemic problems (fix once, in shared code)

These come from the shell, tokens, or shared primitives. Each page inherits them.

### S1. Two visual systems
Live uses `--poke-*` tokens, square fitted bays and one-pixel rules. Runs,
Analytics, Operations, Tools, Media and every dialog use rounded slate cards,
`bg-white/6` fills, `ring-white/10`, and Tailwind `cyan-500`/`rose-500`.
`StatusBadge.vue` and `ConfirmDialog.vue` hard-code the second system, so
any page that uses them drifts too. Result: Live and the rest look like two products.

### S2. No readable type floor, no type scale
Sizes are arbitrary pixel literals (7, 8, 9, 10, 11, 12, 13, 15). Uppercase
tracking labels at 9px act as headings, legends, field labels and status at the
same time (`poke-kicker`, StatusBadge, `text-[9px] uppercase` dt labels). There is
no webfont. The system stack renders differently on each OS, and the mono IDs
fall back to whatever the platform ships.

### S3. Keyboard and assistive access are incomplete
No focus indicators (see table). The live dot is colour + motion only. Rail
thumbnails use `alt=""` even though the frame is the main thing that tells runs apart.
`document.title` changes but view changes aren't announced. Checkpoint rows
aren't controls. `ConfirmDialog` animates scale/translate with no reduced-motion guard.

### S4. Status vocabulary isn't modelled
Each view maps run status → tone separately (`statusTone`, inline ternaries,
`reason === 'done'` checks). Badges are uppercase pills that differ only by colour.
Paused, queued, leased, done, and failed reasons have no stable glyph. Operators
read colour, which also fails S3.

### S5. Shell explains instead of orienting
`App.vue` carries an eyebrow (Monitor, Archive, Telemetry…), a title, and a
sentence-long description for each view. The description renders as an intro block above
the work on every non-Live page. The header mixes fleet counts, wall/UI build
hashes, "Open public" and a filled "New run". The strongest control on the screen
leaves the screen.

### S6. Action hierarchy is flat; destructive safety is inconsistent
Across pages every action is the same small outlined or tinted button. Delete
and Force quit confirm. Cancel run, Clone, and public visibility fire on one
click. Copy and clone receipts are a 1.8 s label swap with no live-region announcement.

### S7. Detail is either fully exposed or hidden in an inline expand
Raw prompts, trace tails, sha256 hashes and `JSON.stringify(checkpoint)` sit in
the primary scan (Live), or entire records open as an inline row expansion that
pushes the table down (Runs). There is no list → detail pattern.

### S8. Navigation state is shallow
Only `#view` and `?run=` are addressable. Filters, sort, sub-tabs (Media), the
selected failure, and the selected inspector item reset on reload or back/forward.
"Inspect" in Runs is a full page reload to `/?run=…#live` (`legacyRunURL`).

### S9. Responsive behaviour is unplanned
Below `xl` the Live rail stacks above the game at `max-h-56`. Runs is a
`min-w-[1220px]` table in a horizontal scroller. The header hides fleet status
below `lg`. There's no drawer pattern and no narrow table pattern.

## Page-specific problems

### Live (`LiveView.vue`, `InspectorPanelInner.vue`, `RunTimeline.vue`)
- **Working well (keep):** Game / Semantic map / Game state triad, unchromed
  pixel frame, party slots with `hp/max` text, Tetris/Boxxle board substitution,
  `ResourceState` honesty (stale banner keeps the last snapshot).
- Programming strip (Live now / Up next / Pause queue) sits *above* the run
  being watched. It's fleet scheduling, not run context.
- A 7-column status strip, then a 4-column fact wall (Settings, Current
  state/Outcome, Plan, Play), then DecisionTelemetry, then "Last event". The same run
  ID, status, location, objective and decision appear 2–3 times.
- The timeline (the chronology the product promises) only shows up inside the
  inspector at the bottom of the scroll.
- Finished replay is a second `<video>` under "Playback" while Game shows "Ended".
- Checkpoints print JSON. There's no restart action (and no API for one; see
  PAGE_REDESIGN §Live/Checkpoints).
- Renderer toggle "Gold / Silver" names a theme, not the mode. 9px controls are
  overlaid on the game frame.

### Runs (`RunArchiveView.vue`, `RunCleanupPanel.vue`)
- "Bulk cleanup" panel renders *above* the archive. The destructive tool comes before
  the data.
- 11 facet selects + success toggle + sort + direction are always visible. Sort
  doesn't use column headers.
- Row = 8 columns, each 3–5 stacked lines of 10–11px text. Badges inside the run
  cell double as filter buttons with no affordance.
- Actions column holds 4 buttons per row (Watch/Generate replay, Details, Inspect,
  Delete). Delete is a one-click-away tinted button on every row.
- The inline "Details" expand dumps 30 key/value pairs in four `dl` columns.
- Replay verb is "Render replay"; the agreed copy is "Generate replay".

### Failures (`FailuresView.vue`)
- Good grouping by triage key. Investigate is filled only when an issue exists,
  and there's no reason shown when it's absent. Example run links truncate IDs to 18 chars.
- Arriving on Live from a failure loses the failure context.

### Analytics (`AnalyticsView.vue`)
- Leads with a 4-up `bg-white/10` KPI tile grid (which DESIGN.md prohibits).
  "Endless experiments" sits here, while paired experiments live in Operations.

### Media (`MediaView.vue`, `RenderJobsPanel`, `BroadcastPanel`)
- Four tabs (Replays, Broadcasts, Render Jobs, Assets/Themes) with intro cards that
  repeat the tab description. Tab state isn't in the URL. Broadcast state
  vocabulary needs the explicit set agreed in the 10-14 spec.

### Operations (`OperationsView.vue`)
- KPI tiles, then Workers, Active attempts (a gameplay table that duplicates Live),
  Queue, Recent outcomes (duplicates Runs). It also hosts LLM deployments and paired
  experiments, which are configuration and analysis.

### Tools (`ToolsView.vue`)
- A single long "New run" form as a top-level nav destination. The success receipt
  (run ID + Open Live) is good and should be kept.

## What to keep (non-negotiable)

1. The triad and its proportions; game pixels stay the most saturated thing on screen.
2. Dark workstation direction, one-pixel rules, tabular numerals, mono machine values.
3. `ResourceState` semantics (loading, error + retry, empty, stale-with-data, refreshing).
4. Every confirm that already exists, plus the copy that names what becomes terminal.
5. All capabilities listed in PRODUCT.md and the 10-14 spec. The redesign moves
   and re-ranks them; it removes none.

## Severity summary

| ID | Issue | Severity | Scope |
|---|---|---|---|
| S3 | No focus indicators, colour-only status | P0 | Global |
| S6 | Unguarded Cancel run beside Pause | P0 | Live |
| S1 | Two visual systems | P1 | Global |
| S2 | 9px/10px text as body | P1 | Global |
| — | Timeline/replay buried under fact wall | P1 | Live |
| S7 | Raw evidence in primary scan / inline row expands | P1 | Live, Runs |
| S5 | Shell intro blocks and header action competition | P2 | Global |
| S4 | Status not modelled | P2 | Global |
| S8 | Shallow URL state | P2 | Global |
| S9 | No narrow-screen patterns | P2 | Global |
| — | KPI tile grids | P2 | Analytics, Operations |
