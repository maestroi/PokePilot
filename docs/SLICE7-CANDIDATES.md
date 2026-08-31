# Slice 7 candidates — raised 2026-08-29

Four gaps raised after slice 6 finished. Recorded so they are not lost; not yet
a plan. Every ROM fact below is cited so the next planner does not re-derive it.

The slice-6 close-out plan (`docs/plans/2026-08-29-slice6-closeout.md`)
is separate and smaller — it repairs debts. This file is about new capability.

---

## 1. The agent never talks to an NPC on purpose

`skill.Talk` and `skill.Face` exist (`skill/interact.go`) and are used for the
nurse, Oak, and the parcel errand. Nothing ever talks to an NPC *to find
something out*. Every NPC that is not on a scripted path is invisible to the
planner.

**Why this is the highest-value item, and not a nice-to-have.** S6-12's design
says, verbatim:

> If the model must learn that Fire loses to Rock, it learns it by losing or
> from an NPC who says so.

There are two legitimate derivation paths and we can currently only take one of
them. The other is a real NPC standing in the gym:

    data/maps/objects/PewterGym.asm
        const_export PEWTERGYM_GYM_GUIDE
        object_event 7, 10, SPRITE_GYM_GUIDE, STAY, DOWN, TEXT_PEWTERGYM_GYM_GUIDE

He is a plain text NPC, not a trainer. So **S6-12's recall-vs-derivation
question is partly unanswerable today**: an agent that reaches the right answer
without ever losing looks like recall, but it may simply have had no way to ask.
Until the guide is reachable, "derivation" is measured with one hand tied.

Note also `PEWTERGYM_COOLTRAINER_M` at (3,6) — the gym has a *second* trainer
before Brock, which no current test accounts for.

**Shape of the work:** a verb that walks to a known NPC and reads what it says,
and a place for what was learned to reach the planner's observation. The
dialogue text decoder already exists (`state.DecodeDialogue`), and S6-8 added
"what the game just told it" to the observation — so this may be mostly wiring,
not new machinery. Check before scoping.

---

## 2. Nothing picks up items, or decides whether to

No item or pickup skill exists in `skill/`. Item balls are `object_event`s with
`SPRITE_POKE_BALL`, collected by facing and pressing A — which is what
`skill.Talk` already does — so the *mechanism* is probably thin. The missing
part is the decision: which items are worth a detour.

**Viridian Forest alone, on the map where the Caterpie hunt already happens:**

    data/maps/objects/ViridianForest.asm
        object_event 25, 11, SPRITE_POKE_BALL, ..., ANTIDOTE
        object_event 12, 29, SPRITE_POKE_BALL, ..., POTION
        object_event  1, 31, SPRITE_POKE_BALL, ..., POKE_BALL

That third one matters immediately. S6-3 measured that **five Poke Balls are not
enough** (~3 per catch, ~13% chance of losing a target outright), and there is a
free sixth ball lying on the floor of the same map. The Potion is worth as much
to the grind: S6-0f's Butterfree/Brock line spends real time on heal detours to
the Viridian Center, and a Potion is a heal that costs no travel.

**Careful:** an item ball that has been picked up keeps a non-zero picture ID
and is suppressed only via `IMAGEINDEX = $ff` — this is already written up in
the slice-5 plan's sprite section. A pickup verb must not re-target a ball that
is already gone, and the blocker overlay must not permanently ban the tile.

---

## 3. Use the swarm to collect failure data

**This is further along than it looks.** `farm/`, `cmd/pokewall` and
`deploy/farm.yml` already exist (commit 07f8e67, "Farm: lease/heartbeat/finish
runner and wall"), and there is substantial *uncommitted* work on `main`
extending it — see the warning at the foot of this file.

S6-12 is the single-box version of exactly this: run the planner N times across
starters and seeds, print a table. The farm turns that from a one-shot
measurement into a continuous harvest.

**The genuinely new part is triage, not running.** Going from "many runs" to
"assign new work to fix the issue" needs failures to be *clustered* — a hundred
runs that all die at the same gate is one bug, not a hundred. S6-9 (failed
objective feedback) and S6-11 (per-objective checkpoints) are the raw material;
what is missing is the grouping.

**One caution, stated plainly:** auto-generating agent-runner tasks from
clustered failures closes a loop from the agent's own failures back into its own
work queue. Slice 6 showed what a bad task costs — three runs stalled or looped,
and one task needed eight attempts because of a single wrong grep. A queue
filled automatically by a noisy classifier would be worse than no queue. Suggest
the swarm *proposes* clusters and a human approves before anything is filed, at
least until the clustering has been seen to be right a few times.

---

## 4. Random starters and seeds

**Mostly already specified — check S6-12 before planning new work here.** Its
task text already requires running "across all three starters and several
seeds", with `-seed` burning idle frames after boot to shift DIV and reroute
every encounter, and it is explicit that different seeds are different *luck*,
not different skill.

There is also uncommitted work on `main` touching `agent/objective.go` and a new
`agent/starter_test.go`, which looks like this item already being started.

So the open question is not "should we randomise" but **"should randomised runs
be continuous rather than a one-off measurement"** — which folds into item 3.
Read S6-12's scoreboard first; if it already answers the capability question,
continuous randomisation is for regression detection, which is a different and
cheaper goal.

---

## Suggested sequencing

1. **Read S6-12's scoreboard.** It decides whether the next slice is about the
   model, the information, or the loop. Nothing here should be planned before
   it is read — and its gate is `-short` only with review off, so read the
   RUNNOTES table itself rather than trusting the green check.
2. **Items (2) before NPCs (1)** if the goal is to make runs succeed: the free
   Poke Ball and Potion are concrete, the mechanism is thin, and both feed the
   measured shortage S6-3 found.
3. **NPCs (1) before the swarm (3)** if the goal is to answer the capability
   question honestly: while the gym guide is unreachable, the recall-vs-
   derivation reading is compromised, and scaling up data collection would
   scale up a measurement we know is compromised.
4. **Swarm (3) and continuous randomisation (4) together**, once there is
   something worth measuring at volume.

---

## WARNING: uncommitted work on `main`

As of 2026-08-29 the `main` working tree carries substantial uncommitted
changes — `.gitignore`, `Makefile`, `agent/objective.go`, `cmd/pokepilot/`,
`cmd/pokewall/`, `deploy/`, `farm/`, plus untracked `cmd/pokeui/`,
`agent/starter_test.go` and several `cmd/pokewall/*_test.go`. It grew during the
night of the 28th–29th while slice 6 ran on `agent-plan/74ab1d4a`.

None of it is committed anywhere. Items 3 and 4 above depend on it. Commit or
stash it before starting slice 7, and before any merge of the slice-6 branch
into `main`.
