# RomPilot Admin — design system

Status: binding visual and interaction system for the operator console
(`web/src/operator`, shared components under `web/src/shared`). The public
spectator (`mode="public"`) is a separate surface and is **out of scope**.

Root `DESIGN.md` keeps the direction contract (what the console *is*). This file
holds the system (how it's built). If they conflict, root `DESIGN.md` wins on
direction and this file wins on tokens, components and patterns. Task T-00 in
the plan adds that pointer.

---

## 1. Direction: "Glass cockpit"

RomPilot is the pilot; the operator is the flight engineer watching the
instruments. The console borrows from avionics glass cockpits: dark,
low-glare fields, hairline rules, tabular readouts, one signal colour for things
you can act on, and colour that means something every time. There is no
ornament. The only vivid imagery is the game itself.

What this rules **in**:
- Fitted instrument bays (panels that share edges and align to one grid), not floating cards.
- Readouts: label + value pairs with mono numerals that don't jitter as values change.
- One filled action per task area. Everything else is outlined or quiet.
- Status as glyph + word + colour, readable in greyscale.

What this rules **out**:
- KPI tile grids, gradient fills, glow, glassmorphism blur, drop shadows on in-flow surfaces.
- Retro pixel fonts or Pokémon-franchise chrome in UI text or controls.
- Intro paragraphs above work. Uppercase micro-labels as headings.
- Colour used only to decorate.

Mode: **Operate**. Scannability and consistency come before expression. The brand
shows in precise details: the readout typography, the status glyphs, the
transport timeline, and the game frame's pixel-perfect well.

---

## 2. Tokens

Tokens live in `web/src/shared/base.css`. Tailwind v4 reads them through
`@theme` so utilities like `bg-surface` and `text-fg-2` exist. Pages never use
raw hex values, Tailwind palette colours (`slate-*`, `cyan-*`, `rose-*`,
`violet-*`, `emerald-*`, `amber-*`), or `white/NN` alpha fills.

### 2.1 Colour

All text tokens were checked at ≥ 4.5:1 on every surface token, and control
borders at ≥ 3:1 (WCAG 2.2 AA, 1.4.3 / 1.4.11).

| Token | Value | Use |
|---|---|---|
| `--canvas` | `#0d1219` | App background, behind bays |
| `--surface` | `#131a24` | Bay bodies, table body |
| `--raised` | `#1a2230` | Bay headers, table header, hover, menus, dialogs |
| `--selected` | `#212c3c` | Selected row/rail item, pressed segmented control |
| `--well` | `#090c11` | Game frame and map wells (letterbox behind pixels) |
| `--rule` | `#253041` | Hairline separators between and inside bays |
| `--rule-strong` | `#3d4a5e` | Bay outer edge where it meets canvas, table header underline |
| `--control-border` | `#6b7a91` | Input / select / outlined-button borders (≥ 3:1) |
| `--fg` | `#e4e8e2` | Primary text, values (14.1:1 on surface) |
| `--fg-2` | `#a9b4c2` | Labels, secondary text (8.3:1) |
| `--fg-3` | `#8996aa` | Tertiary meta, placeholders, disabled text (≥ 4.7:1, all surfaces) |
| `--accent` | `#8fb7e0` | Selection, focus, links, primary action, live position |
| `--on-accent` | `#0b1320` | Text on filled accent (8.9:1) |
| `--ok` | `#7cc496` | Running, success, healthy, goal progress |
| `--warn` | `#dcb06a` | Paused, blocked, stale, checkpoint, needs attention |
| `--danger` | `#e58a8f` | Failed, error, destructive |
| `--danger-solid` | `#b8474e` | Filled destructive button background (white text 5.2:1) |

Each semantic colour gets two derived tokens, defined once and never improvised:
`--ok-bg` / `--warn-bg` / `--danger-bg` / `--accent-bg` = the colour at 12% over
`--surface` (`color-mix(in oklab, var(--ok) 12%, var(--surface))`), and
`--*-rule` at 35%. They're used for inline alerts, the selected-row tint, and pill
backgrounds.

Map tokens (`--map-*`) stay as they are. They describe the semantic map, not the UI.

**Rules**
- The game frame, semantic map and party sprites are the most saturated pixels
  on screen. UI colour never competes in area. No semantic colour fills more than a pill,
  a 2px rule, a meter, or an inline alert.
- Violet is removed. Replay and media use `--accent` like any other action.
- `--accent` means "you can act on this, or it's selected". It never means "info".
- Legacy aliases: `--poke-bg → --canvas`, `--poke-panel → --surface`,
  `--poke-panel-raised → --raised`, `--poke-border → --rule`,
  `--poke-border-strong → --rule-strong`, `--poke-text → --fg`,
  `--poke-muted → --fg-2`, `--poke-dim → --fg-3`, `--poke-cyan → --accent`,
  `--poke-green → --ok`, `--poke-amber → --warn`, `--poke-red → --danger`.
  These stay until the last page migrates, then T-50 deletes them.

### 2.2 Typography

Families (self-hosted variable woff2 in `web/public/fonts/`; no npm dependency):
- **Sans:** IBM Plex Sans (400, 500, 600). Fallback `ui-sans-serif, system-ui, sans-serif`.
- **Mono:** IBM Plex Mono (400, 500). Fallback `ui-monospace, "SFMono-Regular", Menlo, monospace`.
  Used for run IDs, worker addresses, frames, coordinates, timestamps, hashes, seeds, counters.
- `font-variant-numeric: tabular-nums` globally (already set).

Scale (px / line-height / weight). These are the only sizes:

| Token | Size / LH | Weight | Use |
|---|---|---|---|
| `text-meta` | 12 / 16 | 400 | Captions, table secondary lines, timestamps, legends, pill text |
| `text-body` | 13 / 20 | 400 | Default UI text, table cells, form fields, menu items |
| `text-strong` | 13 / 20 | 600 | Emphasised values, row primary text |
| `text-label` | 12 / 16 | 500, `--fg-2` | Field and readout labels, table headers (sentence case) |
| `text-title` | 14 / 20 | 600 | Bay titles, dialog titles, detail-pane headings |
| `text-section` | 16 / 24 | 600 | Page sub-section headings (rare) |
| `text-page` | 20 / 28 | 600 | Page title in the page header |
| `text-readout` | 22 / 28 | 500 mono | Big live readouts (Live "Now" band only: frame, round) |

**Rules**
- **12px is the floor.** Nothing renders smaller, including overlays on the game frame.
- Sentence case everywhere. Uppercase is allowed only for ≤ 4-character
  equipment codes (`HP`, `PP`, `LV`) and keyboard keys.
- No letter-spacing on body text. Labels get `0.01em` at most.
- Text measure: prose blocks (decision rationale, failure detail) cap at 72ch.

### 2.3 Space, size, shape

- **Base unit 4px.** Spacing steps: 4, 8, 12, 16, 24, 32, 48. No other gaps.
- **Bay padding:** 12px horizontal, 8px vertical for header rows. Content
  padding 12px. Dense tables use 12px cell inset horizontally.
- **Control heights:** 28px (`sm`, toolbars and table rows), 32px (`md`,
  default), 40px (`lg`, touch layouts < 768px and dialog primary actions).
  Every hit target is at least 24×24px (WCAG 2.5.8); on touch layouts, 40px.
- **Row heights:** table rows 40px single-line, 56px with a secondary line. Rail rows 64px.
- **Radius:** 0 for bays and tables (they're fitted). 3px for controls, pills,
  and inputs. 6px for floating layers (menus, popovers, dialogs, toasts).
- **Elevation:** in-flow surfaces never have shadows. Floating layers use
  `--raised`, a `--rule-strong` border, and one shadow `0 8px 24px rgb(0 0 0 / 45%)`.
- **Icons:** Heroicons 20/solid at 16px in controls, 20px in empty states. One family only.

### 2.4 Motion

- Durations: 120ms (hover/press, menu open), 180ms (drawer, detail pane), 0ms
  for data updates (values replace; they don't animate).
- Easing `cubic-bezier(0.2, 0, 0, 1)`.
- Allowed ongoing motion: the live dot pulse (1.6s), the timeline playhead,
  and the refresh indicator. Nothing else loops.
- `@media (prefers-reduced-motion: reduce)`: all transitions → 0ms, pulses → static
  filled dot, and the playhead jumps instead of gliding. Enforced globally in `base.css`.

### 2.5 Focus

Global rule in `base.css`:
`:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }`.
Inside fitted bays and table rows, use `outline-offset: -2px` so the ring isn't
clipped. The ring is never removed without a replacement of equal visibility.

---

## 3. Application shell

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ▣ RomPilot   Live  Runs  Failures³  Analytics  Media  Operations   ● 3 live · 2 queued · 1 idle   [+ New run]  ⚙ │  44px
├──────────────────────────────────────────────────────────────────────────────┤
│ Page header (non-Live): Title · context count             [page actions]     │  48px
│ Secondary tabs (when the page has sections)                                  │  36px
├──────────────────────────────────────────────────────────────────────────────┤
│ Page body: full width, 16px gutters (12px < 768px). No max-width on tool pages.│
└──────────────────────────────────────────────────────────────────────────────┘
```

**Top bar (44px, `--canvas` with `--rule` bottom).**
- Brand: 8px accent square + "RomPilot" (14/600). No "Admin" or "Control plane" subtitle.
  The tab title still says "RomPilot Admin · <Page>" to tell it apart from the public site.
- Primary nav, in this order: Live, Runs, Failures, Analytics, Media, Operations
  (per the 10-14 IA). Current item: `--fg` text + 2px accent underline +
  `aria-current="page"`. Failures shows an unacknowledged-group count when > 0
  (pill, text-meta). No other nav badges.
- **Fleet pulse** (right): one link to Operations: `● Connected · 3 live · 2 queued · 1 idle`.
  The dot uses state glyphs (§4.3). When stale it reads `▲ Stale 34s`
  in `--warn`. Wall/UI build hashes move to Settings → About.
- **New run**: outlined `md` button (not filled), opening `#new-run`. It's a
  global utility and doesn't outrank the page's own primary action.
- **Settings**: icon button (gear, `aria-label="Settings"`), opening `#settings`.
- "Open public" leaves the header. It becomes a run-scoped action in Live.
- **< 1024px:** nav collapses behind a menu button. Fleet pulse shrinks to the dot + live count.

**Page header (48px).** Every page except Live has one: `text-page` title, an
optional context count in `--fg-2` (e.g. "1,284 finished"), and right-aligned page
actions (at most one primary). There are **no description paragraphs**.
`descriptions` and `eyebrows` in `App.vue` are deleted.

**Secondary tabs.** Underline tabs, 36px tall, `text-body` 500. Tab state is in the
URL. Used by Media (Library, Studio, Broadcasts, Jobs), Analytics (Overview,
Experiments) and Settings (Models, Destinations, Renderer, About).

**Routing contract.** Hash routes `#<page>[/<section>][?<params>]`. `?run=<id>`
in the search string stays the Live selection for back-compat. Aliases:
`#tools → #new-run`, `#operations` (unchanged), old Media sub-tab links → the
matching section. Filters, sort, pagination, selected item and secondary tab all
go in params, so reload and back/forward restore them. No router library: one
`parseRoute()/formatRoute()` module with unit tests (§ plan T-10).

**Announcements.** One polite `aria-live` region in the shell. It announces page
changes ("Runs, 1,284 finished"), copy receipts ("Run ID copied"), queued actions,
and refresh failures. Toasts are visual mirrors of this region, bottom-right, auto-dismiss
after 4s, and pause on hover/focus.

---

## 4. Components

New shared components live in `web/src/shared/components/ui/`. Each one replaces
an ad-hoc pattern; the replaced pattern is deleted in the same task.

### 4.1 `Bay` (replaces `Panel`)
Fitted region: `--surface` body, `--rule-strong` outer edge, 36px header row on
`--raised` with `text-title` and right-aligned actions. Props: `title`,
`flush` (no body padding, for tables/lists), `as` (section/aside). There's no
`description` prop, because explanatory text isn't a bay feature. Adjacent bays
in a grid share one-pixel rules (`gap-px` on a `--rule` background), as the Live triad already does.

### 4.2 Buttons (`.ui-btn` classes)

Implemented as component classes in `base.css` (`.ui-btn`, `.ui-btn-sm`,
`.ui-btn-primary`, `.ui-btn-quiet`, `.ui-btn-danger`, `.ui-btn-danger-solid`,
`.ui-icon-btn`) on native `<button>`/`<a>` elements. A Vue wrapper would add
nothing. Pills, tags, bays and key/value lists follow the same rule (`.ui-pill`,
`.ui-tag`, `.ui-bay`, `.ui-kv`); only `StatusPill` is a component, because it
maps status values.

| Variant | Look | Use |
|---|---|---|
| `primary` | `--accent` fill, `--on-accent` text | The one main action of a task area |
| `secondary` | transparent, `--control-border` 1px, `--fg` text | Normal actions |
| `quiet` | text only, `--fg-2` → `--fg` on hover, `--raised` hover fill | Toolbar and row actions, tertiary |
| `danger` | transparent, `--danger-rule` border, `--danger` text | Destructive entry points (always opens a confirm) |
| `danger-solid` | `--danger-solid` fill, white text | Only the confirm button inside a destructive dialog |

Sizes `sm` 28 / `md` 32 / `lg` 40. Busy state keeps the width, swaps the label
for the progressive verb ("Pausing…"), sets `aria-busy`, and disables the button.
Disabled buttons that are unavailable for a *reason* show that reason as
adjacent `text-meta` (not only a `title` tooltip).
`IconButton` requires `label` (rendered as `aria-label` + tooltip).

### 4.3 `StatusPill` and the status model
A single function, `presentStatus(kind, value)` in `web/src/shared/status.ts`,
maps every backend status to `{ label, tone, glyph }`. Pages never pick tones
inline. Pill: 20px tall, 3px radius, `--{tone}-bg` fill, `--{tone}` text and
glyph, `text-meta` 500, sentence case.

| Glyph | Tone | Run statuses | Other domains |
|---|---|---|---|
| `●` (pulsing when live) | ok | Running | Worker busy, Broadcast live, Wall connected |
| `‖` | warn | Paused | Queue paused |
| `○` | neutral | Queued, Leased | Job queued, Worker idle |
| `◐` | accent | Starting, Finishing | Job generating (with %), Broadcast starting/reconnecting |
| `✓` | ok | Done — goal complete | Replay ready, Job done |
| `■` | neutral | Done — stopped / ended without goal | Cancelled |
| `▲` | warn | Blocked, Needs attention | Stale data, Checkpoint not restartable |
| `✕` | danger | Failed, Crashed | Job failed, Broadcast failed |

Glyphs are `aria-hidden`; the label carries the meaning. Unknown values render
as neutral `■` with the raw value as label, so new backend states never disappear.

### 4.4 `Tag`
For neutral facts that aren't states: game, planner (Scripted / LLM), play style,
starter, purpose. Outlined 3px radius, `--rule-strong` border, `--fg-2` text. A
Tag used as a filter shortcut is a real `<button>` with a `+` affordance on hover
and focus ("Filter by Pokémon Red"). Max two Tags per row in tables. The rest go to
the detail pane.

### 4.5 `Readout` and `KeyValueList`
`Readout`: `text-label` label above, value below (`text-strong`, or mono for
machine values). `KeyValueList`: two-column `dl`, label column ≈ 40%, rows of
28px, `--rule` separators. Long values wrap; machine values get a `CopyValue`.

### 4.6 `CopyValue`
Mono value + 16px copy `IconButton` labelled "Copy run ID" (or "Copy <noun>").
On click: the icon swaps to a check for 1.5s and the live region announces it. IDs are
**never truncated mid-string without a full-value tooltip and copy**. Prefer
middle truncation (`red-8f2a…c91d`) to tail truncation.

### 4.7 `DataTable` (pattern + thin wrapper)
A `<table>` with standard classes, not a grid framework:
- Header: 36px, `--raised`, `text-label`, sentence case, `--rule-strong` underline,
  sticky inside its scroll container.
- Sortable headers are buttons with `aria-sort`. The direction is shown with a glyph
  (▲/▼). A second click on the active column flips the direction.
- Cells: `text-body`; numbers right-aligned, mono; one primary line and at most one
  `text-meta` secondary line. No third lines.
- Row hover `--raised`. A selected row gets a `--selected` fill + 2px `--accent` left rule
  and `aria-selected`. The whole row opens the detail pane (click, Enter). Checkboxes and the
  row menu stop propagation.
- Row actions: at most one visible quiet action, then a `⋯` `ActionMenu`.
  Destructive items sit last in the menu, after a separator.
- Empty / loading / error use `ResourceState` inside the table frame, so the header stays.
- < 768px: rows render as a two-or-three-line list (same order of facts, no
  cards). The header is replaced by a "Sort" menu.

### 4.8 `FilterBar`
One row: optional search or "Go to run ID" input (240px), up to **four** facet
selects, a `More filters (n)` disclosure for the rest, and a result count on
the right. Active filters show beneath as removable chips with a single
"Clear all". All values go in the URL. Changing a filter resets the page to 1. When rows are
selected, a **bulk action bar** replaces the filter row in place (same height):
"3 selected · [Delete…] · Clear selection".

### 4.9 `ActionMenu`
Headless UI `Menu`. Items: icon + label + optional shortcut hint. Supports
`menuitemcheckbox` for toggles (e.g. "Visible on public site ✓"). Separators
group read actions, then state changes, then destructive actions.

### 4.10 `DetailPane`
Right-side pane for list → detail (Runs, Failures, Operations workers, Media
Library). 440px on ≥ 1280px, overlay sheet below that. It's a region with a heading,
gets focus on open, closes with Esc and returns focus to the row, and is addressable
(`?inspect=<id>`). Header: identity (CopyValue) + status + close. A footer
action bar holds the primary action.

### 4.11 `ConfirmDialog` (restyled; API unchanged + `requireText`)
`--raised` floating layer, 6px radius, max-width 480px, no scale animation
(fade only, 120ms, 0 under reduced motion). Title states the consequence ("Cancel run
red-8f2a?"). The body names what becomes terminal and what doesn't. Cancel is
`secondary` and autofocused for destructive dialogs. Confirm is `danger-solid` with a
specific verb ("Cancel run", "Delete 12 runs"). For bulk deletes of more than 10 items,
`requireText` asks the operator to type the count.

### 4.12 `ResourceState` (kept; restyled)
Same five states, same props. Loading skeletons match the shape of the real
content (table rows, readouts). Error and stale alerts use `--danger-bg`/`--warn-bg`
with the rule colour and a glyph. Refreshing shows a 12px accent dot + "Refreshing"
in the bay header instead of an absolute overlay.

### 4.13 `Tabs`, `SegmentedControl`, `Meter`, `EmptyState`, `Drawer`
- `Tabs`: underline, URL-bound, arrow-key navigation (Headless UI `TabGroup`).
- `SegmentedControl`: 28px, for 2–4 mutually exclusive view modes (e.g. renderer
  Classic / Themed). `--selected` fill on the active segment, `aria-pressed`.
- `Meter`: 4px track `--rule`, fill by tone, always paired with a text value
  (`12 / 40`).
- `EmptyState`: icon 20px `--fg-3`, `text-title` sentence, one-line reason, at most
  one action. Not dashed boxes.
- `Drawer`: left (rail on narrow Live) or right (detail on narrow), 180ms slide,
  focus-trapped, Esc closes.

---

## 5. Layout patterns

| Pattern | Used by | Shape |
|---|---|---|
| **Workspace** | Live | Rail + fitted multi-bay surface filling the viewport; page scroll only below the fold |
| **List + detail** | Runs, Failures, Media Library | FilterBar → DataTable; DetailPane on selection |
| **Overview** | Operations, Analytics Overview | Health strip (one line of readouts, not tiles) → 2-column bays of tables/charts |
| **Form page** | New run, Settings sections | Single column ≤ 720px, grouped fieldsets, sticky action footer |

Grid: 12px gaps between bays, 16px page gutters. Bays align to the same left edges
across rows. Within a page, nothing nests more than **two** levels (page → bay →
content). A bay never contains another bay.

## 6. Density

The console is dense by default. Density is a property of the typography floor and
row heights above, not of shrinking text. Rules:
- Show the minimum fields that answer the page's question. Everything else goes in the detail pane.
- A table row has one primary line and at most one secondary line.
- Lists over 50 items paginate server-side (existing APIs) or virtualise.
  They never expand inline.

## 7. Copy

Follow root DESIGN.md §Copy. Additions:
- Verbs: "Generate replay" (not Render), "Run again with same settings" (not Clone),
  "Start a new run from here", "Investigate with AI", "Copy run ID".
- Renderer modes: "Emulator" (authoritative framebuffer) and "Themed" (semantic
  renderer, theme picker inside). Never name a mode after a game.
- Time: relative in tables ("12 min ago") with an absolute ISO `title`. Durations
  are `1h 04m`. Frames use thousands separators.
- Errors: what failed + what still works + what to do. Raw backend messages go into
  a "Details" disclosure.

## 8. Accessibility checklist (every task)

- Visible `:focus-visible` on every interactive element (§2.5).
- All status uses glyph + text (§4.3). Nothing is colour-only.
- Live regions for receipts and refresh failures. The rail is a `listbox`
  with arrow keys. Timeline markers are buttons with `aria-label` describing the event.
- Game frames have meaningful `alt` ("Game frame, red-8f2a, Viridian City").
  The semantic map canvas exposes the player position in its accessible name.
- Contrast per §2.1. Hit targets per §2.3. Reduced motion per §2.4.
- Every dialog traps focus and restores it on close.

## 9. Responsive

| Width | Behaviour |
|---|---|
| ≥ 1440 | Full layouts; Live rail 248px, Runs DetailPane docked |
| 1280–1439 | Live rail 216px; DetailPane docked at 400px |
| 1024–1279 | Live rail collapses to a 72px thumbnail strip (expand on focus/hover-intent button); DetailPane becomes overlay sheet |
| 768–1023 | Nav behind menu; Live triad becomes Game + Map row, Game state below; rail becomes drawer |
| < 768 | Single column; tables become lists; controls `lg` (40px); Live order: run header → Game → Now → Map → Game state → timeline (horizontal pan) → story |

No horizontal page scroll at any width ≥ 320px. Only the timeline and wide
tables may scroll horizontally inside their own region, with a visible scroll affordance.
