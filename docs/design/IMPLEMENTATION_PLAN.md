# RomPilot Admin redesign — implementation plan

Implements [DESIGN.md](DESIGN.md) and [PAGE_REDESIGN.md](PAGE_REDESIGN.md).
Issue map: epic #2670; shell #2672, Operations/Settings #2673, Analytics/Experiments
#2674, Runs #2675, Media #2676. Each task below is one PR, unless it says
"bundle with". Link the matching issue in the PR.

## Ground rules (apply to every task)

- **No API or business-logic changes.** Tasks move, re-rank and restyle existing calls.
  A control that needs an API that doesn't exist is out of scope (marked **[needs API]**
  in PAGE_REDESIGN). File an issue for it instead of shipping a stub.
- **Public spectator isolation.** Don't change `mode="public"` rendering. `npm run build:spectator`
  (including `verify-public-theme-build.mjs`) must pass unchanged.
- **Delete what you replace** in the same PR (old component, old classes, dead helpers).
- **Standard verification `V`** (run from `web/`):
  `npm test && npm run typecheck && npm run build:operator && npm run build:spectator`.
- **Visual check `VIS`**: run `npm run dev` against the dev backend. Capture the touched
  page at 1440×900 and 375×812, attach both to the PR next to the matching
  `docs/ui-before/` capture, and tab through the touched region once to confirm visible focus.
- **Design ratchet.** After T-02, `test/designSystem.test.ts` fails on banned
  patterns in any operator file not on its `LEGACY` allowlist. Each page task removes its
  files from the allowlist, and the list may only shrink.

---

## Phase 0 — foundations (no visible page redesign yet)

### T-00 Baseline and pointers
- Add the eight current-page screenshots to `docs/ui-before/` (Live, Runs, Failures,
  Analytics, Media, Operations, Tools, plus Live at 375px).
- Root `DESIGN.md`: add one line under the title pointing to `docs/design/DESIGN.md`
  for tokens/components/patterns.
**Accept:** files present; root DESIGN.md links to docs/design/DESIGN.md; no code changes.

### T-01 Tokens
- `web/src/shared/base.css`: add every token from DESIGN §2.1 / §2.3 / §2.4 on
  `:root`, expose colours, radii and font sizes through Tailwind v4 `@theme`, and redefine
  the `--poke-*` variables as aliases of the new tokens (DESIGN §2.1 legacy aliases).
- Add the type scale utilities `text-meta`, `text-body`, `text-label`, `text-title`,
  `text-page`, `text-readout`.
**Accept:** `V` passes. `rg -- '--poke-(bg|panel|text|cyan)' web/src/shared/base.css`
shows only alias definitions. Live renders with no layout change (VIS). Colour shifts
are limited to the documented token deltas.

### T-02 Accessibility floor + design ratchet
- `base.css`: global `:focus-visible` rule (DESIGN §2.5); global
  `prefers-reduced-motion` reset (DESIGN §2.4); `body` font size 13px / 20px line height.
- `ConfirmDialog.vue`: drop the scale/translate transition (fade only).
- New `web/test/designSystem.test.ts`: walk `src/operator/**/*.vue` and
  `src/shared/components/**/*.vue` (excluding spectator-only components:
  `AppShell` public branch is exempt via a marker comment). Fail on
  `text-\[(7|8|9|10|11)px\]`, `\b(slate|rose|violet|emerald|cyan|amber)-\d{2,3}\b`,
  `rounded-(md|lg|xl|full)` on non-pill elements, `bg-white/`, `ring-white/`,
  for files not in an exported `LEGACY` set. Seed `LEGACY` with every current offender.
**Accept:** `V` passes. Tabbing through the current Live page shows a focus ring on every
control. With OS reduced-motion on, no transitions or pulses run. Adding `text-[9px]`
to a non-legacy file makes `npm test` fail (check locally, don't commit).

### T-03 Fonts
- Self-host IBM Plex Sans (variable, latin) and IBM Plex Mono (400/500) woff2 in
  `web/public/fonts/` with `@font-face` + `font-display: swap` in `base.css`.
  Apply to the operator only (scope under `.operator-root`, set by `App.vue`), so the
  spectator keeps its own fonts.
**Accept:** `V`; operator network tab shows only the 2–3 font files, < 150 KB total;
the spectator build output contains no reference to the Plex files.

### T-04 Status model
- New `web/src/shared/status.ts`: `presentStatus(kind, value)` per DESIGN §4.3 for
  kinds `run`, `outcome`, `worker`, `job`, `broadcast`, `replay`, `connection`.
  Unknown values → neutral `■` + raw label.
- New `web/test/status.test.ts` covering every listed backend value + an unknown value.
- New `ui/StatusPill.vue`. Replace `StatusBadge` usages **in shared components only**.
  Pages switch in their own tasks.
**Accept:** `V`; test covers paused, queued, leased, running, done-with-goal,
done-without-goal, failed, an unknown value; every pill renders glyph + text.

### T-05 Core controls
- New `ui/Button.vue`, `ui/IconButton.vue` (variants/sizes per DESIGN §4.2),
  `ui/Tag.vue`, `ui/CopyValue.vue` (+ middle truncation helper with a unit test),
  `ui/Readout.vue`, `ui/KeyValueList.vue`, `ui/Meter.vue`.
- Restyle `ConfirmDialog.vue` onto `Button` and add the `requireText` prop.
**Accept:** `V`; `ConfirmDialog` and these components pass the ratchet (not in
`LEGACY`); middle-truncation test passes; IconButton without `label` is a type error.

### T-06 Containers and states
- New `ui/Bay.vue`. Make `Panel.vue` a thin wrapper rendering `Bay` (ignoring
  `description`) so pages migrate gradually, and delete it in T-50.
- Restyle `ResourceState.vue` per DESIGN §4.12 (same props).
- New `ui/EmptyState.vue`, `ui/Tabs.vue` (URL-bound via T-10 helpers; ship
  after T-10), `ui/SegmentedControl.vue`, `ui/ActionMenu.vue`, `ui/Drawer.vue`,
  `ui/DetailPane.vue`, `ui/Toast.vue` + shell live region composable `useAnnouncer()`.
**Accept:** `V`; every new file passes the ratchet; `DetailPane` and `Drawer`
trap focus, close on Esc, and restore focus (manual check in VIS on any page).

### T-07 Table and filter primitives
- `ui/DataTable.vue` (thin wrapper: sticky header, sortable header buttons with
  `aria-sort`, selected-row styling, narrow-list slot) and `ui/FilterBar.vue`
  (≤ 4 visible facets, More filters disclosure, active chips, bulk-bar slot).
**Accept:** `V`; a sortable header toggles `aria-sort` between `ascending` and
`descending`; FilterBar hides facets beyond the 4th behind "More filters (n)".

## Phase 1 — shell and navigation (#2672)

### T-10 Route model
- New `web/src/operator/route.ts`: `parseRoute(location)` / `formatRoute(route)` for
  `#<page>[/<section>][?params]`, aliases (`#tools → new-run`, legacy media
  sub-tabs), and `?run=` back-compat. `useRoute()` composable syncing with
  `hashchange` + `history.pushState`.
- `web/test/route.test.ts`: round-trip, aliases, unknown page → live, param encoding.
**Accept:** `V`; all alias cases tested; existing `mediaNavigation.test.ts` still passes.

### T-11 App shell
- Rebuild the operator branch of `AppShell.vue` / `App.vue` per DESIGN §3: brand,
  6-item nav, fleet pulse linking to Operations, outlined New run, Settings icon,
  shell live region, page header component (`ui/PageHeader.vue`). Delete
  `descriptions`, `eyebrows` and the intro block for private mode. Move wall/UI
  build provenance to a temporary About popover (Settings arrives in T-13).
- "Open public" leaves the header (re-added in Live T-21).
**Accept:** `V`; VIS. Nav has 6 items + Settings + New run. No intro paragraph on any
page. `#tools` opens New run. Page changes are announced (verify with the live region in
devtools). At 375px the nav is in a menu and the fleet pulse shows a glyph + live count.

### T-12 New run page
- `#new-run` renders `ToolsView` restyled as a Form page (PAGE_REDESIGN §3): fieldsets,
  advanced disclosure, sticky "Queue run" footer, success receipt with `CopyValue`
  + "Open in Live" (in-app navigation).
**Accept:** `V`; VIS; queueing a scripted and an LLM run still sends the same
`createRun` payload as before. Compare request bodies in devtools before and after on
the same inputs and paste them in the PR. Remove ToolsView from `LEGACY`.

### T-13 Settings destination (#2673)
- `#settings` with tabs Models (`LLMDeploymentsPanel` moved from Operations),
  Destinations (broadcast destination config, if currently in BroadcastPanel), Renderer
  (Media › Assets/Themes moved), About (build provenance from T-11).
**Accept:** `V`; VIS; deployment create/test/save/delete and worker-cap patch behave
as before (manual run-through listed in the PR); Operations no longer renders deployments.

## Phase 2 — Live workspace (reference screen)

Do these in order. Each one leaves Live fully working.

### T-20 Live: rail
- Rail per PAGE_REDESIGN §1.3A: groups with counts, 64px rows, one pill + ≤ 1 tag,
  meaningful `alt`, `listbox` keyboard model, `j`/`k`, programming footer (moved
  from the top strip; same API calls).
**Accept:** `V`; VIS. Arrow keys move the selection and Enter selects (updates `?run=`).
The programming strip no longer renders above the workspace. Skip/Retry/Pause queue
still call the same endpoints.

### T-21 Live: run header and action hierarchy
- Run header per §1.3B with the state → action table, `ActionMenu`, Cancel run
  confirm, Run-again toast, public visibility `menuitemcheckbox`, Open public page,
  `c`/`p` shortcuts, action errors under the header.
**Accept:** `V`; VIS. For running, paused, queued, ended-with-replay and ended-without-replay
fixtures, exactly one filled button renders. Cancel run opens a dialog and
`cancelRun` is only called on confirm (add a unit test for the handler if it's
extracted to a `.ts` module; otherwise do a manual check in the PR). No shortcut calls a
destructive API.

### T-22 Live: triad polish + Now band
- Triad bays on `Bay`, renderer `SegmentedControl` (Emulator | Themed) with a theme
  popover, 12px floor in all overlays and party slots.
- Now band replaces the status strip **and** the Settings / Current state / Plan /
  Play fact wall. Planner content moves into an interim disclosure until T-24.
**Accept:** `V`; VIS. At 1440×900 the run header, triad and Now band are above the fold.
Run ID, status, objective and decision each appear once above the fold.
`rg 'text-\[(7|8|9|10|11)px\]' src/operator/LiveView.vue` is empty.

### T-23 Live: timeline transport
- Move `RunTimeline` directly under the Now band at full workspace width. Filter
  `select` replaces the chips. Markers are labelled buttons, with `[`/`]` stepping and
  Follow live toggle. Event selection state lives in the URL (`?event=`).
**Accept:** `V`; `runTimelineFilters.test.ts` passes; VIS: selecting a marker turns
Follow live off and Follow live snaps back. On 375px the track pans inside its own region
with no page-level horizontal scroll.

### T-24 Live: story + inspector
- Story list synced to the timeline selection. Inspector tabs Event / Planner /
  Checkpoints / Artifacts / Run details per §1.3F. `InspectorPanelInner` is split into
  these tabs and its JSON / sha256 move behind disclosures. Checkpoints show the
  "isn't available yet" note (no restart button, **[needs API]**).
**Accept:** `V`; VIS. No `JSON.stringify` output is visible without opening a "Raw"
disclosure. Selecting a story entry selects the same timeline marker and the reverse.
Tabs survive reload (`?tab=`).

### T-25 Live: replay in the Game well
- Watch replay swaps the Game well to the replay video, controlled by the transport.
  Map/state show the honest "Final state" label (or a labelled snapshot). Delete the
  inspector Playback `<video>`. Generate replay / generating / failed states per §1.4.
**Accept:** `V`; VIS on a finished run with a ready replay. Only one `<video>` element
exists in the DOM. Seeking the transport seeks the video. The "Final state" label is visible.

### T-26 Live: responsive + states sweep
- Breakpoints per §1.5 (thumbnail strip, rail drawer, stacked order) and every state
  in §1.4, including the unknown `?run=` alert.
**Accept:** `V`; VIS at 1440, 1280, 1024, 768, 375. No horizontal page scroll at
375. The page-level acceptance list in PAGE_REDESIGN §1.6 passes item by item (tick the
list in the PR). Remove LiveView, RunTimeline and InspectorPanelInner from `LEGACY`.

## Phase 3 — Runs (reference data-heavy screen, #2675)

### T-30 Runs: URL state + FilterBar + views
- All filters, sort, direction, page and `inspect` go through `useRoute()`. FilterBar with
  Game/Outcome/Model/Goal visible and the rest under More filters. Active chips. Views row
  (All / Successful / Fastest successful). "Go to run ID" via `getRun`.
**Accept:** `V`; a `runArchiveQuery` round-trip test (route → archive API params →
route) passes. VIS: reload and back/forward restore filters and page; ≤ 4 facets visible.

### T-31 Runs: table
- Columns per §2.3 on `DataTable`, sortable headers + "Sort by" menu, Tags as filter
  buttons, Replay cell states, row `ActionMenu`. Delete only in the menu.
**Accept:** `V`; VIS. Every row is ≤ 2 lines. Sorting by header changes the `sort`/`dir`
params and the request. "Generate replay" calls `renderReplay`. A generating replay links
to Media › Jobs with the job selected.

### T-32 Runs: detail pane
- `DetailPane` with §2.3 sections. It replaces the inline Details expand, which is deleted.
  ↑/↓ moves the selection while the pane is open. Inspect in Live navigates in-app.
**Accept:** `V`; VIS. `?inspect=` opens the pane on reload. Esc closes and focus returns
to the row. No document reload when choosing Inspect in Live (check Network → Doc).

### T-33 Runs: selection, bulk delete, bulk cleanup drawer
- Bulk bar replaces the FilterBar when rows are selected. The delete dialog lists the
  IDs, with `requireText` above 10. Partial-failure toast. `RunCleanupPanel` moves into a
  Drawer opened from the page header.
**Accept:** `V`; existing `runCleanup.test.ts` and `runArchiveSelection.test.ts` pass.
VIS: cleanup isn't rendered on page load. Deleting 11 selected runs needs the typed
count. The dialog shows "and N more" beyond 5 IDs.

### T-34 Runs: narrow layout + states
- List rendering < 768px, the overlay sheet below 1280, and every state in §2.4.
**Accept:** `V`; VIS at 1280, 1024, 768, 375. PAGE_REDESIGN §2.6 checklist ticked in
the PR. Remove RunArchiveView and RunCleanupPanel from `LEGACY`.

## Phase 4 — remaining pages (spec each in PAGE_REDESIGN before starting)

Each starts by extending PAGE_REDESIGN §3 for that page into a full spec (same
depth as §1/§2) in the same PR. Each ends by removing its files from `LEGACY`.

| Task | Page | Issue | Key acceptance |
|---|---|---|---|
| T-40 | Failures | #2672 | Full IDs with CopyValue; Live shows the "From failure group" strip with a back link; Investigate disabled-with-reason when no issue |
| T-41 | Operations | #2673 | Health strip (no tiles); Active attempts and Recent outcomes tables removed; worker run links open Live in-app |
| T-42 | Analytics + Experiments | #2674 | No KPI tile grid; paired experiment creation moved from Operations and still posts the same `createExperiment` body |
| T-43 | Media (Library, Studio, Broadcasts, Jobs) | #2676 | Tab + selected item in URL; on-air strip only when the backend state is live; legacy media links resolve to Jobs with the job selected |

## Phase 5 — close-out

### T-50 Remove legacy
- Delete the `--poke-*` aliases, `Panel.vue`, `StatusBadge.vue`, `.poke-kicker`, and the
  `table[class~="min-w-[1220px]"]` hack. The `LEGACY` set is empty and removed.
**Accept:** `V`; `rg -- '--poke-|poke-kicker|StatusBadge|<Panel' web/src/operator web/src/shared/components`
returns only spectator-scoped hits (none expected in the operator).

### T-51 Accessibility and contrast audit
- Run axe (Playwright `@axe-core` via `npx`, no new dependency in `package.json`) on every
  page at 1440 and 375. Run a keyboard-only pass through Live and Runs.
**Accept:** zero serious/critical axe violations. Findings and fixes are listed in the PR.
`docs/ui-after/` holds the matching captures for every `docs/ui-before/` image.

---

## Sequencing and dependencies

```
T-00 → T-01 → T-02 → {T-03, T-04, T-05} → T-06 → T-07
                      T-10 → T-11 → {T-12, T-13}
T-05,T-06,T-11 → T-20 → T-21 → T-22 → T-23 → T-24 → T-25 → T-26
T-07,T-10 → T-30 → T-31 → T-32 → T-33 → T-34
T-26,T-34 → T-40..T-43 → T-50 → T-51
```

Live (Phase 2) and Runs (Phase 3) can run in parallel once Phases 0–1 land.

## Out of scope / open decisions

- **Checkpoint restart** ("Start a new run from here"): needs a backend endpoint and
  paired-knowledge restartability flag. File it under #2670. The UI slot is reserved in T-24.
- **"Has linked issue" Runs view**: needs an archive filter param.
- **Free-text run search**: not supported by the archive API. "Go to run ID" covers
  exact lookup.
- **Command palette (⌘K)**: deferred. Add it after T-51 if keyboard navigation telemetry or
  operator feedback asks for it.
