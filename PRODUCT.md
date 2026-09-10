# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

The private PokéFarm console is for the operator developing and running
PokePilot. They use it while runs are active and after runs finish to understand
what the game and planner did, find failures, reproduce them, and hand evidence
to a debugging agent.

The public spectator is a separate read-only audience and security boundary.

## Product Purpose

PokePilot plays Pokémon games through semantic objectives and deterministic
game controls. The operator console makes each run understandable and
debuggable: follow the game live, inspect decisions and semantic game state,
review completed runs, replay what happened, restart from a durable checkpoint,
and investigate failures.

Success means an operator can answer these questions quickly:

- What is the run doing now, and why?
- Where did progress stop or fail?
- What game state and evidence explain that result?
- Can I replay the run or restart from the right checkpoint?
- What run ID and evidence should a debugging agent investigate?

## Positioning

The console joins the actual Game Boy output with PokePilot's semantic view of
the same run: map and player state, planner decisions, structured objective
outcomes, checkpoints, deterministic recordings, and debugging artifacts.
Gameplay truth comes from decoded game state; the screen exists so a human can
watch and correlate that truth with what happened visibly.

## Operating Context

The main workflow begins with active runs. The operator selects a run, watches
the live game and semantic map, follows its current objective and latest planner
decision, and scans progress over time. A completed run reopens in the same
workspace with a scrollable history and replay controls.

Debugging starts from the point of failure. The operator can inspect structured
outcomes and raw evidence, render or seek a deterministic replay, choose a
replayable checkpoint, start a new run from it, copy the run ID, or hand the
failure to an AI investigation flow. Fleet health, run creation, failure groups,
and worker details support this workflow but are not the primary screen content.

## Capabilities and Constraints

- The private console can queue, cancel, inspect, replay, restart, delete, and
  investigate runs through allowlisted PokePilot APIs.
- Live game frames, semantic maps, player position, movement trails, party,
  money, badges, planner state, structured outcomes, artifacts, checkpoints,
  and replay status already exist in the product.
- Historical telemetry is honest about its limits. The current backend does not
  provide a semantic decision event for every historical video frame.
- `run.gbrun` is the canonical deterministic recording; generated video is a
  disposable seekable derivative.
- A checkpoint can restart an LLM run only when its paired agent knowledge is
  available and replayable.
- ROM, save-state, infrastructure, raw model, and debugging details remain
  private. The public spectator stays a separate sanitized surface.
- The interface must remain usable on desktop and narrow screens, with keyboard
  access, visible focus, readable status without relying on color alone, and
  reduced-motion support.

## Evidence on Hand

- Live and finished run data from the PokéFarm dashboard.
- Run debug bundles, structured objective outcomes, trace tails, and progress
  snapshots.
- Game frames and embedded semantic map assets.
- Checkpoint state and paired knowledge metadata.
- `run.gbrun` recordings and generated replay video.
- Artifact hashes, runner revisions, model statistics, and linked issue state.

No testimonials, marketing claims, or decorative product imagery are needed for
the operator console.

## Product Principles

1. The selected run is the workspace; fleet administration supports it.
2. Show the game, the semantic state, and the planner decision together.
3. Present a run as a chronological story that is easy to scan and replay.
4. Put debugging actions beside the evidence and moment they act on.
5. Keep summaries readable; expose raw data on demand without duplicating it.

## Accessibility & Inclusion

The console must support keyboard navigation, clear focus, semantic controls,
status text in addition to color, readable contrast, and reduced motion.
