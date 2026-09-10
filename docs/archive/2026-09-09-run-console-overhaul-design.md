# Run Console Overhaul Design

**Status:** approved direction and composition

## Problem

The private PokéFarm console gives run selection, live watching, history, run
inspection, failure triage, and fleet administration similar visual weight. A
selected run is split between the watch pane and a separate inspector nested
inside history. Copy controls turn readable text into form-like fields. Run
outcomes lack a clear chronological relationship to decisions and game state,
and checkpoint restart is separated from the moment it restarts.

The result is difficult to scan during a live failure and difficult to revisit
afterward.

## Audience and Job

The primary user is the PokePilot operator watching current runs, finding where
they fail, replaying completed runs, restarting experiments, and handing a run
ID plus evidence to an AI debugging flow. This is an **Operate** surface.

The selected run is the workspace. Fleet administration supports that work.

## Selected Direction

Use a broadcast-control-room structure without broadcast terminology. One run
is the dominant working feed, its Game and Semantic Map views are paired, other
runs remain visible as compact sources, and one transport timeline controls live
following and historical review.

The visual system is defined in `/DESIGN.md`. It uses restrained dark equipment
surfaces, precise separators, tabular machine data, and limited semantic status
colors. The game frame supplies most of the visual color.

The approved composition uses the watching-first second mockup: the Game and
Semantic Map pair dominate the upper workspace, current state forms one compact
strip beneath them, the timeline spans the full selected run, and Run Story is
the main reading surface below. Carry forward the first mockup's expanded
failure row with contextual evidence and actions.

Add the first mockup's compact system-operations summary and top-level tabs.
`Live` opens the run workspace; `Runs` opens searchable history; `Failures`
opens grouped actionable defects; `Operations` opens workers, services, and
versions; `Tools` holds run creation and secondary utilities. The main Live
view retains only a small health summary so operational status is visible
without competing with the selected run.

## Information Architecture

### Global bar

Show product identity, `Live`, `Runs`, `Failures`, `Operations`, and `Tools`
tabs, compact farm health, connection state, and one `New run` action. Detailed
worker versions and infrastructure facts move to Operations.

### Run rail

Show active runs first, then recent runs. Each item carries status, a small frame
preview when available, goal or current objective, location, and elapsed or end
time. The full run ID is available from the selected-run header rather than
repeated as a dominant label on every row.

The console opens the active run, then the last selected run, then the most
recent failed or completed run.

### Selected-run workspace

The header gives a human-readable run state, goal, status, location, and a
single copyable run ID. The main visual pair is:

- **Game** — the live frame or seekable generated replay;
- **Semantic Map** — the map, player, sprites, trail, and selected-event context.

Current objective and latest AI decision stay immediately adjacent to this
pair. Party, badges, money, and current progress remain visible as compact game
state rather than a separate dashboard card collection.

### Timeline and playback

One horizontal transport strip represents the run from start to current or final
frame. It merges only persisted facts:

- planner decisions and objective boundaries;
- battles and map transitions when recorded;
- checkpoints with paired-knowledge availability;
- badge or other progress markers;
- recovery, blockage, and failure outcomes;
- replay video position.

Live mode follows the newest event until the operator scrolls or selects an
older marker. A `Return to live` control appears when following is paused.
Completed runs use the same strip as replay transport. The video can seek by
frame, while semantic detail snaps to the nearest persisted marker and labels
that relationship honestly.

### Run Story

A continuous chronological list below the timeline replaces the detached
history/inspector combination. Each compact entry shows round or frame, decision
or objective, structured outcome, meaningful game-state change, and location.
Entries expand in place for error text, planner exchange, artifact links, and
other evidence. Selecting an entry selects its timeline marker and vice versa.

### Evidence and actions

Technical evidence opens in one focused drawer or expanded Run Story entry. It
contains raw planner exchanges, trace, artifact metadata/downloads, hashes,
runner revision, model statistics, and debug JSON.

Actions live beside their scope:

- `Generate replay` and replay status beside transport controls;
- `Start a new run from here` on replayable checkpoint markers, with round,
  objective, and paired-knowledge status shown before submission;
- `Investigate with AI` beside a failure summary, naming the run ID and evidence
  bundle being handed off;
- cancel on an active-run header;
- delete in the run-level overflow menu.

### Operations

Queueing runs, worker/fleet health, aggregate failure groups, and versions move
to a secondary Operations view or drawer. Failure groups can deep-link into the
representative run and selected failure event.

## State and Error Behavior

- Loading preserves the workspace skeleton and existing selected run.
- A wall disconnect clearly freezes data and marks its age.
- Replay uses explicit `Unavailable`, `Ready to generate`, `Generating`,
  `Ready`, and `Failed` states.
- A checkpoint explains why it cannot restart when state or paired knowledge is
  absent.
- Missing historical semantic data is shown as unavailable rather than inferred
  from nearby video frames.
- Long errors are summarized in Run Story and expanded without turning them into
  editable-looking text areas.
- Empty active runs fall back to recent runs with a clear empty message.

## Backend and Data Boundary

The first implementation composes existing dashboard, run, debug, checkpoint,
artifact, frame, replay-status, replay-video, and triage APIs. It does not require
a new event stream to establish the new hierarchy.

Where the current backend only exposes summary markers, the timeline remains
sparse and honest. A future semantic event stream may increase marker density
without changing the workspace model.

## Accessibility and Responsiveness

All run rows, timeline markers, disclosure controls, and actions are keyboard
operable with visible focus. Status never relies only on color. The timeline has
an equivalent ordered list through Run Story. Reduced-motion mode removes live
playhead animation.

Below desktop width, the run rail becomes a drawer, Game and Map stack, the
timeline scrolls horizontally, and expanded evidence appears inline.

## Verification

- DOM/contract tests pin the major regions, truthful labels, and action
  placement without mirroring implementation markup.
- Unit tests cover timeline ordering, marker selection, live-follow pause/resume,
  checkpoint eligibility, and outcome labeling.
- Existing operator API proxy, replay, checkpoint, and triage tests remain
  green.
- Browser verification covers desktop, narrow viewport, keyboard navigation,
  loading/disconnected states, one active run, multiple active runs, a completed
  replayable run, and a failed run with checkpoints and AI investigation.
- A live farm smoke test confirms frame updates, map updates, replay generation,
  checkpoint restart, and run deep links.

## Explicit Non-goals

- Changing the public spectator surface in this redesign.
- Fabricating frame-level semantic history from video.
- Changing PokePilot gameplay or planner policy.
- Uploading ROM or save-state data to the browser.
- Rebuilding the console in a frontend framework solely for this change.
