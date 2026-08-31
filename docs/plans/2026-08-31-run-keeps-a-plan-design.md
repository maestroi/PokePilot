# A Run That Keeps a Plan — Design

**Date:** 2026-08-31
**Status:** DRAFT — for discussion, nothing agreed. Written the night before so
the conversation starts from a shape rather than a blank page. Every "Decision"
below is a *proposal*; the Open Questions are the real agenda.

## Context

`agent.Run` carries three things between rounds: the `Observation` (present
tense by construction), a `Knowledge` (what has been *seen*), and `History`
(what happened, last `historyCap` = 6 rounds). None of them is a plan. Every
round the model answers "what now?" from a rebuilt menu, greedily, forever.

`docs/SLICE9-CANDIDATES.md` §2 named this: *"There is no slot anywhere for a
plan. That is the gap, and it is goal-agnostic."*

Slice 9's answer was `Intent` — one sentence the model attaches to its choice,
carried onto the next observation with an `IntentAge`. **MEASURED 2026-08-31,
it does not close the gap**, because the model writes the sentence *after*
picking, so it describes the pick instead of constraining it:

    choice 15 -> go to viridian city   "Travel to Viridian City to reach the Gym..."
    choice 23 -> go to pokemon center  "Heal Bulbasaur before challenging the Gym."
    choice 19 -> go to viridian mart   "Buy a Potion before heading to the Gym."

Each is a fine justification of the objective it just chose. None survives to
constrain the next round; the sentence is rewritten to match whatever gets
picked, so `IntentAge` sits near zero and never does the job it was built for.
It is a caption, not a commitment.

### What it costs, measured

The best run to date (2026-08-31, qwen3.8-27b thinking off, `-max-rounds 200`)
reached Route 2 and Viridian Forest in 23 rounds. Rounds 12–18 of that run:

    12: go to route 1          16: go to route 1
    13: train to level 10 (retreat)   17: talk at (5,24)
    14: train to level 11 (retreat)   18: talk at (15,13)
    15: heal at the center

**~7 of 23 rounds** circling Route 1 and the Viridian Center. At ~10s of
emulator time per round that is ~70s of a ~4-minute run spent going nowhere,
and the fraction grows with run length.

## Goal

A run can hold an objective longer than it can remember, and spends its
expensive reasoning once per plan instead of once per round.

## The shape proposed

Two jobs at two cadences, which today are collapsed into one:

- **Strategist** — thinking ON, called *rarely*. Given the goal, the
  observation and the known world, it emits an **ordered list of objectives**.
- **Execution** — no model at all. `Run` takes the plan's next step, confirms
  it is on this round's menu, and runs it.

The model is consulted again only when the plan actually breaks.

### Decisions (proposed)

- **Plan steps are objective SENTENCES, not menu indices.** An index is
  meaningless next round — the menu is rebuilt every time, and this session
  already measured an entry moving from index 9 to 17 as the known world grew.
  `agent.Chosen` already resolves a sentence against the offered list
  (`strings.ToLower(o.String()) == want`), so a plan of sentences slots into
  what exists. This is also why `Objective.String()` must stay stable — it is
  now load-bearing for three things: `Knowledge.Completed`, the persisted
  memory file, and plan-step resolution.

- **A step that no longer resolves is a re-plan trigger, not an error.** The
  world moved; that is information, not a fault.

- **Re-plan triggers are the design.** Too eager and the run pays 45s
  constantly; too reluctant and it marches into a wall it should have noticed.
  Proposed set: plan exhausted · next step not offered · the step failed ·
  a blackout · a newly stated requirement (`Knowledge.Requirements` grew).

- **Thinking is a per-call flag, already.** `LLMPlanner.NoThink` is per-request
  (`agent/llm.go`), so "think for this call, not that one" needs no new
  plumbing — only a rule for *when*, and that rule IS the trigger list above.

- **The plan is run memory, like `History`.** It belongs beside `Knowledge` in
  the checkpoint/memory file so a resumed run does not forget what it was
  partway through. Memory file is at v4; this would be v5.

- **`Offer` does not change.** It still says what is POSSIBLE, never what is
  wise, and the plan is still drawn only from what it offers. The safety
  property is unchanged: the model can never invent an action.

### The cost arithmetic

Using this session's measured numbers (qwen3.8-27b served locally):

| | per call | per 8 rounds |
|---|---|---|
| greedy, thinking off | ~1.2s | ~10s |
| strategist, thinking on | ~45s | ~45s (once) |

So a plan of 8 steps costs ~35s more model time and buys back ~7 wasted rounds
at ~10s of *emulator* time each. Net positive at today's numbers, and it
improves with run length: wasted rounds scale with the run, re-plans do not.

**This arithmetic is the weakest part of the design and should be attacked
first tomorrow.** It assumes plans average ~8 useful steps before breaking. If
they break after two, the whole thing is a 45s tax for nothing.

## Open questions — the actual agenda

1. **How long is a plan before it breaks?** Unknown, and it decides
   everything. Cheapest way to find out without building the feature: take the
   23-round run's actual objective sequence and ask, by hand, how many steps a
   strategist could have committed to at round 1. If the answer is 3, redesign.

2. **What does the strategist see that the pilot doesn't?** If it gets the same
   observation, is a 45s think actually better than a 1.2s one — or is the win
   entirely from *commitment* rather than from *reasoning*? A cheap ablation:
   plan with thinking OFF. If a no-think plan is as good, the thinking half is
   optional and this gets much cheaper.

3. **Does a bad plan do more damage than greedy wandering?** A committed run
   executes 8 steps without asking. If step 2 was wrong, greedy would have
   noticed at round 3 and a plan will not. The failure tally and located walls
   (both landed 2026-08-31) are the inputs that could catch it — but nothing
   currently reads them mid-plan.

4. **Should the plan be visible to the strategist as its own prior?** Showing
   the previous plan and how far it got risks the model rubber-stamping it, the
   way `Intent` gets rubber-stamped today. That failure mode is measured and
   should be assumed to recur unless designed against.

5. **Where does this leave `Intent`?** A plan supersedes what Intent was for.
   Keep both, or does the plan become the intent?

## What this does NOT do

- It does not give the run cross-run memory. `Knowledge` is still per-run
  (`SLICE9-CANDIDATES.md` §3). Deliberately separate — a run that inherits
  notes is measuring a different question, and that line has been held
  everywhere else in this project.
- It does not fix skill-level stoppers. The current hard stop is a wedged
  trainer battle (`SLICE10-CANDIDATES.md` item 19), which no amount of planning
  reaches.
- It does not write any Pokémon knowledge into Go. The plan is drawn from the
  offered menu; nothing seeds it.

## Rejected, with reasons

- **Making `Intent` binding** (reject the pick if it does not serve the stated
  intent). It inverts the wrong half: the model would learn to write intents
  that match whatever it wanted to pick, which is what it already does.
- **A fixed objective sequence for the Boulder Badge.** That is writing the
  answer down, which voids the measurement the project exists to make.
- **Removing repeated objectives from `Offer`.** Withholding legal options is
  us playing the game; the menu now annotates counts instead (commit 184f28f).

## Prerequisites

- **S10-3's number.** The monitoring task reports the planning-vs-skill failure
  ratio. If most stoppers are skill bugs, this design is premature and the
  slice should stay on `skill/`.
- **Nothing else.** `Chosen`, `Offer`, the memory file and the per-call
  thinking flag all already exist.
