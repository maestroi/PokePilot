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
Direction: restrained broadcast control room using plain PokePilot labels. Use the watching-first second mockup, the first mockup's tabs and compact system health, the third mockup's precise checkpoint marker interaction, and the first mockup's compact party state.
Memorable moment: selecting a failure joins the visible game moment, semantic location, structured outcome, evidence, Investigate with AI, and Start a new run from here in one expanded Run Story row.
Constraints: keep public spectator separate; no fabricated frame-level semantic history; no ROM/state exposure; keyboard and narrow-screen support; no framework migration solely for this redesign.
Open decisions: none that block implementation.
