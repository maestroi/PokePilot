# Slice 6 — badge 1 with any starter: the parcel, the bag, and catching

Status: **draft plan in agent-runner** — `74ab1d4a-c27e-495e-a329-0f06f8ef296f`.

**The runner plan is the source of truth.** Task instructions, measured ground
truth and verification commands live there. This file is the map: what the
slice is for, what order the tasks go in, and why. If the two disagree, the
runner plan wins and this file is stale.

Depends on `docs/SLICE5-CLOSEOUT-PLAN.md` (runner plan
`db45f28c-0998-4c13-8c00-25349535c694`) finishing green.

## The goal, and why it is this goal

Obtain the Boulder Badge with **every** starter, repeatably across seeds — not
once, with the one starter that happens to walk through Brock.

Squirtle beats Brock at level 8. Bulbasaur beats him trivially. Charmander
**cannot**, and that is a property of the game rather than of our policy: Rock
resists Fire, and Red has no easy Fighting answer. The real line is to catch a
Caterpie in Viridian Forest and bring a Butterfree.

So "badge 1 with any starter" is the smallest goal that forces every mechanism
the rest of the game needs: an item errand, a shop, a bag, catching, and a
party bigger than one. An agent that only completes Squirtle runs has overfit,
and that is the capability question this project exists to answer.

## What reshaped this slice

Slice 5's audit found that catching is not one feature. A second party member
makes two code paths reachable that have **never executed**, and both are
questions the game asks that the code currently answers by tapping A:

- the nickname prompt after a successful catch (`AskName`,
  `pokered/engine/menus/naming_screen.asm`);
- "Use next MON?" plus the party menu when the active mon faints with a live
  reserve (`DoUseNextMonDialogue`, `ChooseNextMon`,
  `pokered/engine/battle/core.asm:1052`).

Nothing in the tree can tell a question from a statement. `Cutscene`,
`advanceUntil` and `Battle`'s `default:` branch all press A and rely on the
yes/no defaulting the way they want. The one predicate that exists —
`yesNoMenuUp` — is safe only because its two callers use it as a *wait* ("has
the menu I expect appeared?"), never as a *classifier*.

That is why three prerequisite tasks were added ahead of the original list, and
why the forced switch after a faint became a required part of S6-5 rather than
an UNMEASURED aside.

## Task order

Numbering is historical — the runner plan's positions are authoritative, and
there is no S6-1.

| # | Task | Depends on |
|---|---|---|
| 1 | **S6-0a** One predicate that tells a question from a statement | — |
| 2 | **S6-0b** Recover from an interrupted text box without answering a question | S6-0a |
| 3 | **S6-0c** Give every map its name | — |
| 4 | **S6-2** The bag in battle — `UseItem` from the fight menu | S6-0a |
| 5 | **S6-3** `skill.Catch` — spend balls until the party grows | S6-2, S6-0a |
| 6 | **S6-4** Survive evolution and the move-learning prompt | S6-0a |
| 7 | **S6-5** Lead order, voluntary switch, forced switch after a faint | S6-0a, S6-3 |
| 8 | **S6-6** Buy from a mart | S6-3's balls-per-catch measurement |
| 9 | **S6-7** Objectives take arguments; the model picks its own starter | — |
| 10 | **S6-8** Moves, bag, recent dialogue and history in the observation | S6-0c |
| 11 | **S6-9** A failed objective is information, not the end of the run | S6-8 |
| 12 | **S6-10** Offer what the player could plausibly know | S6-9 |
| 13 | **S6-11** Checkpoint every objective | — |
| 14 | **S6-12** Let the model play it, score it, diagnose why it failed | everything |

S6-0a is the keystone. Four later tasks have to answer a yes/no, and without it
each hand-rolls its own guess.

## The rules that already cost this project time

- A predicate asserts something POSITIVE about the state you want, never merely
  the absence of what you do not want.
- Battle menus are identified from `wTileMap`, never `wFontLoaded` — MEASURED:
  `wFontLoaded` stays 0 for an entire battle.
- `wTextBoxID` is not a liveness bit. It reads `0x01` before, during and after
  ordinary dialogue, and every catch leaves a stale `0x14` behind.
- Menus are step-and-verify: press, assert `wCurrentMenuItem`, then A. Never a
  press count, never a frame count.
- Coordinates come from `skill.Place`, never literals.
- Measure by hand first, put the numbers in the task text, leave the worker an
  edit. A task that says "find out why" spends its whole budget finding out.
- A stated fact that turns out to be false must be REPORTED, not worked around.

## Scope

**In:** the choice predicate, dialogue recovery, map names, the bag, catching,
evolution and move-learning prompts, party order and switching, buying,
aimable objectives, a richer observation, failure feedback, knowledge-gated
offers, checkpointing, and the scoreboard.

**Out:** Mt. Moon and badge 2, HMs and field moves, the session UI, and any
Nuzlocke-style perturbation beyond the starter.

**Deferred, deliberately:** structured `ObjectiveResult` with objective-level
postconditions. `agent/objective.go` still has four `fmt.Printf` calls carrying
gameplay outcomes (travel blackout, training blackout, Gym win, Gym loss) that
should be structured return data. S6-9 makes failures reach the planner as
text, which is enough for this slice's question. Revisit when a second consumer
needs the outcome as data rather than a log line.
