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

## Measured: plan lifetime (S11-2, 2026-09-01)

Attacked the arithmetic by hand, on the recorded 23-round run. The full
per-round trace survived at `/tmp/pokepilot-run-llm-fixtest2.log`
(qwen3.8-27b, `POKEPILOT_LLM_NO_THINK=1`, `-max-rounds 200`, seed 0,
`stopped: failed after 23 round(s)`). It matches the rounds 12–18 quoted
above verbatim. The run was invoked without `-checkpoint-dir`, so that log is
the whole recorded evidence — no per-round states exist for it.

**The number: a plan committed at round 1 lives 1 step under this design's
own safety property, ~4 under the loosest defensible reading, and from
round 12 it cannot reach the goal at all.** The ~8-step assumption is not
supported. Details and the measured/inferred split follow.

### The recorded sequence (measured — straight from the log)

| r | objective picked | outcome | menu size |
|---|---|---|---|
| 1 | take the bulbasaur starter | done | 5 |
| 2 | deliver oak's parcel | failed — blackout | 16 |
| 3 | deliver oak's parcel | failed — blackout | 15 |
| 4 | deliver oak's parcel | done | 15 |
| 5 | go to route 1 | done | 17 |
| 6 | go to viridian city, fleeing | done | 15 |
| 7 | go to viridian pokemon center | done | 24 |
| 8 | heal the party | done | 19 |
| 9 | go to viridian mart | done | 19 |
| 10 | buy 3 POTION | failed — not in stock | 18 |
| 11 | talk at (3,3) | done | 18 |
| 12 | go to route 1, fleeing | done | 19 |
| 13 | train the lead to level 10 | failed — retreat (ended L9) | 17 |
| 14 | train the lead to level 11 | failed — retreat (ended L9) | 21 |
| 15 | heal at VIRIDIAN POKEMON CENTER, fleeing | done | 21 |
| 16 | go to route 1, fleeing | done | 19 |
| 17 | talk at (5,24) | done | 17 |
| 18 | talk at (15,13) | done | 18 |
| 19 | go to viridian city | done | 17 |
| 20 | go to route 2 | done | 24 |
| 21 | go to pewter city, fleeing | failed — wedged trainer battle (0x33) | 19 |
| 22 | pick up the POTION at (12,29) | failed — same battle | 26 |
| 23 | pick up the POKEBALL at (1,31) | failed — same battle | 26 |

The menu size changes in **22 of 23 rounds** (5,16,15,15,17,15,24,19,19,18,18,
19,17,21,21,19,17,18,17,24,19,26,26). The menu is not a stable surface a plan
can be drawn from; it is rebuilt and re-composed every round.

### Walking forward from round 1

**What Offer actually showed at round 1 (measured from `agent/offer.go`):**
5 objectives — the three starters (offered only while `PartyCount == 0`) plus
two variants (fight / flee) of a journey to the single known place beside the
starter screen. `deliver oak's parcel` (`KindErrand`) is gated on the
rival-in-lab event and is NOT on the round-1 menu; it first appears at round
2 (16 offered). No journey to Viridian, Pewter or the gym is on the round-1
menu either — those maps are not yet in `knownMaps`.

- **Strict reading — plan ⊆ the round-1 Offer (the design's stated safety
  property: "the plan is still drawn only from what it offers", "the model can
  never invent an action").** The only goal-directed step available is
  `take the bulbasaur starter`. That is **1 step**. The instant it executes,
  `PartyCount` becomes 1, the starters drop off the menu, the errand appears,
  and the menu jumps to 16 — the plan is exhausted and nothing else in it can
  still resolve. **Plan lifetime = 1.**
- **Loosest defensible reading — plan is a list of sentences, each re-resolved
  against the menu when its turn comes, and the strategist may commit a step it
  has parametric reason to expect (not one on the round-1 menu).** Walking the
  recorded sequence: (1) take starter — offered r1, resolves; (2) deliver
  oak's parcel — offered r2, resolves (the two blackouts are the game
  answering, not a plan break); (3) go to viridian city — offered r6,
  resolves; (4) heal / go to viridian pokemon center — offered r7–8, resolves;
  (5) `train the lead to level N` — offered from r13 but **returns an error**
  (retreat: "continuing would have lost it"), and "the step failed" is one of
  this design's own re-plan triggers. **The plan breaks at step 5; lifetime ≈
  4.** And even those 4 do not approach the goal — the next steps the goal
  needs (`go to route 2 → go to pewter city → go to pewter gym → beat the
  leader`) only resolve once the run has actually walked to Route 2 and Pewter,
  which the plan cannot force.

Either way the honest number is **1–4, not 8**.

### Walking forward from round 12 (the realistic mid-run case)

At the start of round 12 the player is in the Viridian Mart; the menu has 19
entries. Reconstructing `knownMaps` from the Offer rule (`{current map} ∪
visited ∪ adj(current) ∪ dialogue-named places`) with the visited set
{Pallet 0x00, Lab 0x1c, Route 1 0x0c, Viridian City 0x01, Center 0x29, Mart
0x2a}: the offerable places are Pallet, the Lab, Route 1, Viridian City, the
Center and the Mart, plus local verbs (train, heal, talk, buy, catch, pickup,
use-item). **`go to route 2`, `go to pewter city` and `go to pewter gym` are
not on the menu** — Route 2 (0x0d) and Pewter (0x02) are neither visited nor
adjacent to the mart/Route 1, so they are not in `knownMaps`. (Inferred from
the Offer rule + the visited set; consistent with the model first reaching
Route 2 at r20 and Pewter at r21.)

So a plan drawn from the round-12 menu **cannot contain the goal.** It can only
re-state the local circling the greedy loop was already doing (train, heal,
go to route 1, talk) — and even that is broken, because the `train` step
keeps returning the retreat error, which is a re-plan trigger. **A mid-run plan
here buys back zero of the 7 circling rounds**, because those rounds were not
wasted on a lack of commitment: the goal-relevant objective was simply not on
the menu to commit to.

### Why the circling happened (measured facts, inferred cause)

1. **The goal was mis-located.** The goal is the **Boulder Badge = Brock =
   Pewter City Gym** (`pewter gym`, map 0x36, in the place table). There is **no
   `viridian gym` in the place table** (measured: `skill/goto.go` lists only
   `pewter gym` and `cerulean gym`). Yet the model's intent strings say "the
   Viridian Gym" / "Saffron Gym route" for most of the run — it was grinding
   toward a gym that is not enterable this early and not even a named place.
2. **The route to the real goal was outside the known world** until the run
   walked Route 2 (r20). Until then the menu could not offer it, so neither a
   greedy pick nor a committed plan could take it.
3. **`train` kept retreating** (lead ended L9, "continuing would have lost
   it"), a skill/content stopper that no plan schedules around.

None of the three is fixed by holding an ordered list of offered objectives.
(1) and (2) are knowledge/world-model gaps; (3) is a skill defect. The design
explicitly refuses to seed Pokémon knowledge ("nothing seeds it"), so as
specified it cannot close (1) or (2).

### Measured vs inferred

- **Measured:** the per-round objective/outcome/menu-size table (the log);
  the place table (no `viridian gym`; `pewter gym` = 0x36); the Offer gating
  rules (starter only at `PartyCount==0`; errand gated on the rival event;
  journeys only to `knownMaps`; gym only on a gym map); the re-plan trigger
  list; `memoryVersion = 4`; that adjacency is full route geometry, not
  game-shown.
- **Inferred:** the exact menu *contents* per round (the log records only the
  size; contents reconstructed from the Offer rule + the visited set);
  that Pewter/Route 2 were not offered during r12–18 (consistent with the
  model first reaching them at r20/r21); the 1 / ~4 / 0 lifetime figures
  (walking the recorded sequence under the measured Offer rules).

### Recommendation: **redesign, do not build as specified**

The design's cost arithmetic assumed ~8 useful steps; the measured lifetime is
1 (strict) to ~4 (loose), and from the realistic mid-run point the plan cannot
reach the goal at all. A 45s thinking call that buys back 1–4 rounds at ~10s
of emulator time each is a wash-to-tax, not a win, and the win does not improve
with run length the way the doc claims, because the waste here is a world-model
gap that scales with the run exactly as fast as the re-plans do. Building the
strategist as designed would instrument a mechanism that cannot fix the
measured failure. The thing to build first is getting the **goal into the known
world** (the run must know the Boulder Badge is in Pewter and that Route 2
leads there) — a seeding/knowledge decision this doc currently declines to
make — and fixing the `train` retreat stopper. Revisit the plan only once the
menu actually contains the goal; at that point a plan's job shrinks to
sequencing local steps, which is cheaper to get from the existing loop.

### The two questions the same evidence decides

- **Does the plan supersede `Intent`, or do both survive?** The plan, if it
  worked, occupies exactly the slot `Intent` was built for ("what am I trying
to do", carried across rounds) but as a commitment instead of a caption. They
  are the same slot with two implementations, not two needs. **Decision: the
  plan supersedes `Intent`; do not carry both into the memory file's v5.**
  Because the recommendation is to redesign before building, the concrete move
  now is to add *no* plan slot to v5 yet — and when a (re)designed plan lands,
  it replaces `Intent` rather than joining it. (Inferred from the measured
  `Intent` behaviour in §Context, not from a new measurement.)
- **Open question 4 — would showing the strategist its previous plan invite the
  rubber-stamping already measured for `Intent`?** Yes, and it should be
  assumed to recur. `Intent` is stamped because the sentence is written *after*
  the pick, so it describes the pick. A strategist shown its previous plan plus
  "how far it got" faces the same shape: the cheap move is to re-emit the old
  list, and in this run the plan breaks early and often, so "how far it got" is
  usually "barely" — which argues for repeating, not re-deriving. Design
  against it: on a re-plan, show the **break point and what changed** (which
  trigger fired: step not offered / step failed / blackout / new requirement),
  not the old plan as a prior to continue, and ask for a fresh plan conditioned
  on the change. (Inferred from the measured `Intent` failure mode; the
  rubber-stamp itself is not yet measured for a plan, because no plan exists.)

## Open questions — the actual agenda

1. **How long is a plan before it breaks?** **ANSWERED (S11-2, above): 1 step
   under the design's own safety property, ~4 under the loosest reading, and 0
   toward the goal from round 12.** The answer is below the "if 3, redesign"
   line, so the doc's own rule says **redesign**.

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
