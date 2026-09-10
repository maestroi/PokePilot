# Game-Native Operator Console Redesign

**Status:** approved  
**Supersedes:** visual and composition details in
`2026-09-09-run-console-overhaul-design.md`

## Problem

The current console has the desired shell—a top navigation bar, run rail, and
selected-run workspace—but its visual language reads as a generic dark
operations dashboard. Pokémon identity is too weak because real game imagery,
party state, maps, badges, and run events are visually subordinate to neutral
panels and aggregate cards.

Completed-run playback is also structurally duplicated. The Game bay shows a
last frame while generated video appears later in the timeline inspector, even
though the live image is no longer useful once a run has ended.

Operations mixes fleet control with campaign analytics. Six equal KPI cards,
badge distributions, terminal outcomes, and endless-run comparisons do not help
an operator answer the immediate operational questions: which workers are
healthy, what each worker is doing, what is queued, and what just ended.

## Approved References and Direction

Use the supplied dark PokePilot console references as visual authority:

- the active/failed-run composition and expanded failure row from the
  `exec-34da...` reference;
- the completed-run replay, evidence rail, and long-form timeline from the
  `exec-e438...` reference;
- compact party state and persistent fleet health from the `exec-3c99...`
  reference.

The result is a serious, dense operator console. Pokémon identity comes from
authentic game frames, semantic map tiles, species portraits, party state,
badges, encounters, and event symbols. It does not come from themed plastic,
pixel fonts for normal text, bright franchise colors, or decorative nostalgia.

## Visual System

- Near-black and blue-black fields define the shell and fitted work areas.
- Fine cool-gray rules establish hierarchy without floating card shadows.
- Cyan/blue identifies selection, live position, and primary operator actions.
- Green means confirmed progress or healthy state.
- Amber marks checkpoints, blocked states, and recoverable attention.
- Red is reserved for failure and destructive action.
- UI copy uses a compact workhorse sans; run IDs, frames, coordinates, and
  timestamps use tabular monospace.
- Corners remain square or lightly rounded. Dense tables, aligned strips, and
  fitted bays are preferred over interchangeable cards.

## Selected-Run Workspace

Keep the current top navigation, left run rail, and dominant selected-run
workspace.

The run rail shows active runs first and recent runs after them. Each useful row
includes a game thumbnail or species portrait when available, status, concise
goal/location, and elapsed or end time. The selected row uses a clear cyan edge
and field rather than a large saturated background.

The first workspace row contains:

1. **Game** — live image or finished-run replay;
2. **Semantic Map** — map, player, sprites, route trail, and event context;
3. **Game State** — compact party, badges, money, and relevant final state.

A dense strip below the visual row contains objective, latest decision or
outcome, location, frame/time, and compact party summary. Information should not
be repeated in separate generic cards.

## Unified Game and Replay Bay

The Game bay owns both live and historical visual playback.

- Active run: show the live frame pump.
- Finished run with no generated video: show the last frame and one
  `Generate replay` action.
- Generating: preserve the last frame and show progress/status in the Game bay.
- Ready: replace the image with the replay video and native transport controls.
- Failed generation: keep the last frame and provide a local retry action with
  the actual error.

The separate inspector video is removed. Timeline event selection seeks the
video when possible. The semantic map and details use the nearest persisted
semantic event and explicitly say when they are not frame-exact; the interface
must not invent historical state for arbitrary video frames.

## Timeline and Run Story

Use one compact timeline beneath the visual workspace. It represents persisted
decisions, objectives, battles, checkpoints, progress, recovery, and failures.
Video time and semantic events share the same transport without claiming a
semantic record for every frame.

Run Story is a dense chronological table. The selected event expands in place
to show structured outcome, concise evidence, artifact links, checkpoint
eligibility, and scoped actions:

- `Investigate with AI` for an actionable failure;
- `Start a new run from here` for a replayable checkpoint;
- evidence/artifact disclosure for technical detail.

## Operations and Analytics

Operations becomes a fleet-control surface:

- system and service health;
- worker slots, addresses, revisions, last-seen age, and current assignments;
- queued and leased work;
- active attempts;
- recent terminal outcomes and direct links to their runs.

Campaign analytics move to a dedicated `Analytics` view:

- objective completion;
- badge distribution;
- terminal outcomes;
- retry failures;
- endless-experiment comparisons.

This reuses the existing stats endpoint and changes information architecture,
not gameplay/runtime semantics.

## Technical Approach

Keep the existing framework-free frontend. A Vue migration is not justified by
the current interaction complexity and would add delivery and test risk without
improving the result.

Refactor the accumulated CSS override layers into one coherent token and layout
system. Split rendering by surface where it reduces coupling, while preserving
the existing backend endpoints and private/public security boundary.

No backend change is required for the initial redesign. Missing species artwork
or historical semantic snapshots degrade honestly to current text/last-state
content.

## Responsive and Accessible Behavior

- At narrow widths the run rail becomes a drawer.
- Game, map, and game state stack in that order.
- Timeline remains horizontally navigable and Run Story provides the equivalent
  ordered event list.
- All markers, rows, disclosures, and actions are keyboard operable.
- Focus is visible, status never depends on color alone, and reduced motion is
  honored.

## Verification

- Contract tests pin unified Game/replay placement, the Analytics tab, and the
  fleet-focused Operations regions.
- Replay tests cover missing, generating, ready, error, and retry states.
- UI behavior tests cover selecting a finished run, generating replay, seeking
  from an event, returning to live, and switching Operations/Analytics.
- Existing proxy, checkpoint, artifact, triage, and replay backend tests remain
  green.
- Browser review covers active, completed, failed, disconnected, empty, and
  narrow-screen states against the supplied references.

## Non-Goals

- Migrating to Vue or another framework solely for this redesign.
- Changing the public spectator.
- Fabricating frame-level semantic history.
- Changing gameplay, planner policy, or artifact security.
