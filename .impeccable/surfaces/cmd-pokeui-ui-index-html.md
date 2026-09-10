---
version: 1
slug: "cmd-pokeui-ui-index-html"
primary_target: "cmd/pokeui/ui/index.html"
related_targets: ["cmd/pokeui/ui/ui.js","cmd/pokeui/ui/inspector.js","cmd/pokeui/ui/stats.js"]
---

Scope: private operator console at cmd/pokeui/ui/index.html; Operate mode.
Audience and job: the PokePilot operator watches current runs, finds where they fail, reviews completed runs, replays behavior, restarts from a checkpoint, and hands exact evidence to AI investigation.
Primary task: one selected run dominates; Game, Semantic Map, objective, decision, timeline, and Run Story explain what is happening and why.
Proof and content: real dashboard state, game frames, semantic maps, structured outcomes, checkpoints with paired knowledge, deterministic replay, artifacts, runner revision, and issue links.
Direction: restrained game-native workstation using plain PokePilot labels. Identity comes from real game frames, semantic maps, party state, badges, encounters, and persisted event symbols—not retro chrome or decorative franchise styling. Use a near-black/blue-black shell with fine cool rules, cyan selection/action, green progress/health, amber checkpoints/attention, and red failure/destructive action. Compact sans copy and tabular mono values sit in fitted bays and aligned tables rather than generic cards.
Composition: Game and Semantic Map dominate the selected-run workspace with a compact third Game State bay and six party slots. Finished replay replaces live media inside Game. The selected run and event use a subtle cyan field with a one-pixel accent.
Information architecture: Operations is fleet control—health, workers, queue/leases, and recent outcomes. Analytics owns campaign aggregates.
Memorable moment: selecting a failure joins the visible game moment, semantic location, structured outcome, evidence, Investigate with AI, and Start a new run from here in one expanded Run Story row.
Constraints: keep public spectator separate; no fabricated frame-level semantic history; no ROM/state exposure; keyboard and narrow-screen support; no framework migration solely for this redesign.
Open decisions: none that block implementation.
