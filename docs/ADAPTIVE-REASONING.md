# Adaptive reasoning — investigation, 2026-09-01

Question asked: how should PokePilot balance fast non-reasoning inference
against slow reasoning inference?

**Read `docs/plans/2026-08-31-run-keeps-a-plan-design.md` first.** It already
proposes the two-cadence strategist/execution shape, and its Open Questions
are the real agenda. This document does not replace it. What is new here is
§6 (rewiring the existing stuck/failure detectors from *stop* to *escalate*,
which ships without any plan at all), the correction in §8 that per-call
thinking is **not** free plumbing, the skip-vs-replan rule in §5, and §9's
metrics.

**Short answer, before the detail.** PokePilot already has the architecture
the proposal describes. Deterministic code owns navigation, battles, menus
and shops; the model already picks a semantic objective from a rebuilt menu,
never a button press; `NoThink` is already a per-request flag and its cost is
already measured (47.1s → 0.88s on the same model). What is missing is not a
fast tier. It is that **every round asks the same question from scratch, and
every "the run has gone wrong" signal is wired to STOP instead of to
ESCALATE.**

So the recommendation is not fast/slow inference. It is three tiers where the
cheapest one is **no model call at all**, and where the existing stuck/failure
detectors become replan triggers instead of exit codes.

---

## 1. Current architecture

| Layer | Package | What it owns |
|---|---|---|
| Emulator | `emu/` | frames, input, save states, the sample hook |
| ROM facts | `red/rom/`, `red/sym/` | maps, warps, collision, move table, map headers |
| Game state | `red/state/` | RAM → typed state: party, bag, badges, player, menus, dialogue |
| Geometry | `world/` | map graph, collision grids, BFS pathfinding |
| Verbs | `skill/` | boot, starter, goto, travel, battle, gym, shop, heal, train, catch, pickup, field items |
| Intent | `agent/` | `Observation`, `Offer`, `Objective`, `Execute`, `Run`, `LLMPlanner` |
| Binaries | `cmd/` | `pokepilot` (run + watch), `badgerun` (scoreboard), `pokewall`/`pokeui` (farm) |

The pieces that matter for this question:

- **`agent/observe.go`** — `Observation` is the entire view a planner gets:
  map, position, party, badges, money, bag, lead's moves, events, respawn
  point, wild-encounter table for this map, `HasGrass`, map objects from the
  ROM header, plus run-owned fields (`History`, `Failures`, `Requirements`,
  `RecentDialogue`, `Round`/`RoundsLeft`, `Intent`/`IntentAge`).
- **`agent/offer.go`** — `Offer(obs, known)` rebuilds the legal menu every
  round from the observation plus `Knowledge` (maps stood on, place names the
  game has spoken, completion counts, failure tally, walls heard verbatim,
  people already talked to, ROM adjacency). It offers what is *possible*,
  never what is *wise*, and annotates each line with `(done 3x, failed 2x)`.
- **`agent/objective.go`** — 11 `Kind`s. `Execute` dispatches to `skill`.
  `Validate` range-checks every argument before any input reaches the
  emulator.
- **`agent/llm.go`** — one POST per round to an OpenAI-compatible
  `/chat/completions`, temperature 0, `response_format: json_schema`
  constraining the reply to `{choice, intent}`. Envelope is verified (model
  field, `finish_reason`) before content is parsed. `NoThink` sets
  `chat_template_kwargs: {"enable_thinking": false}`.
- **`agent/run.go`** — the loop, the budgets, the stuck/failure accounting,
  the checkpoint ring, the dialogue tape, and the between-round dialogue
  recovery.
- **`cmd/pokepilot/stats.go`** — `statsPlanner`, a decorator around
  `LLMPlanner` that tallies calls, repeats, latency and tokens and pushes
  `farm.LLMStats` to the watch page and the farm heartbeat.

### Thinking mode today

`LLMPlanner.NoThink` (`agent/llm.go:86`) is a struct field set once from
`POKEPILOT_LLM_NO_THINK`, so it is **per-run, not per-call** — but the
mechanism underneath it is already per-request: it becomes a field on the
request body. Flipping it per call requires no new plumbing at all.

The measurement is already in the code comment, and it is the whole
motivation for this investigation:

> qwen3.8-27b, one 16-objective menu: thinking on, 47.1s / 4096 completion
> tokens, truncated mid-thought and REJECTED as `finish_reason "length"`;
> thinking off, 0.88s / 22 tokens, a clean `{"choice": 1, ...}`.

### Persistence between calls

Three things survive a round: `Knowledge` (in memory, serialised beside every
checkpoint by `agent/memory.go`), the scrolling `History` (6 rounds), and the
model's own `Intent` sentence + `IntentAge`. `Run` carries the intent
verbatim and deliberately never writes or summarises it — that was S9-7's
measurement design, and it is the only strategic state that exists.

---

## 2. Current decision flow

One round, traced:

1. `Run` folds the last round's observation into `Knowledge` (`SawMap`,
   `SawDialogue`), plus every map the dialogue tape saw the objective walk
   through (`run.go:521`).
2. `known.Requirements` and `known.FailureList()` are copied onto the
   observation; `Round`/`RoundsLeft`/`Intent`/`IntentAge` are set.
3. `Offer(last, known)` builds the menu — verbs first, journeys last
   (measured: journeys-first buried "deliver oak's parcel" at index 17 of 17).
4. `planWithRetries` → `LLMPlanner.NextFeedback` → one POST. Observation is
   marshalled as compact JSON; the menu is a 1-based numbered list of
   `Objective.String()` with notes.
5. Reply → envelope check → `resolveReply` → `Chosen(offered, "N")` →
   `WithArgs` (applies/rejects `intent`). A rejected reply re-asks the *same*
   round with the rejection quoted back, up to `MaxReplyRetries = 3`.
6. Checkpoint written **before** `Execute` (the state the decision was made
   in), with the knowledge file beside it.
7. `Execute` → `skill.Travel` / `skill.Train` / `skill.Gym` / … Deterministic
   code does pathfinding, battle turns, menu navigation, dialogue paging,
   re-planning after every leg.
8. `observeAfter` pages away any leftover text box, then `Observe`.
9. Outcome recorded in `History` and `Knowledge`; `sameProgress` compares
   map/position/party-count/events to bump or reset the `stuck` counter.

**One model call per round. No model call anywhere else in the system.**

---

## 3. Problems

1. **Every round re-derives strategy.** The model is asked "what next?" with
   a 6-round scrolling history and a 200-byte intent slot, at temperature 0.
   Measured on the best run to date (2026-08-31, 27B local, thinking off):
   **~7 of 23 rounds were spent circling Route 1 and the Viridian Center** —
   route 1, train, train, heal, route 1, talk, talk. Every one of those rounds
   was a legal objective that succeeded. The greedy loop's cost is invisible
   line-by-line and only shows in the tally.

2. **Latency is charged per round, and it scales with the menu.** Measured
   2026-08-29: ~6-7s per call at 5-7 offered objectives, ~21-23s at 13-15.
   The menu is combinatorial (item 17): one catch per wild species, one
   use-item per medicine × affected slot, one *pair* of go-to entries per
   known place. It grows with every map discovered, so latency grows as the
   run succeeds.

3. **All the "something is wrong" detectors terminate instead of escalating.**
   `StopStuck` (3 rounds of unchanged observation), `StopFailed` (3
   consecutive failures, or the same objective failing the same way twice),
   `retreatStreak`, `ErrReplanExhausted`, `MaxReplyRetries` exhaustion — every
   one of these ends the run. The run that reached Viridian Forest stopped at
   round 23 of a 200-round budget. **The budget was never the limit.**

4. **Some rounds are guaranteed-wasted and deterministic code already knows
   it.** `Offer` sells POTION at a mart that does not stock it (item 20). It
   offers Train to a lead that `Train` will refuse (item 1). Each is a model
   call plus an emulator excursion spent to learn something the ROM could have
   told us for free.

5. **The one strategic slot has been measured, and it does not work.**
   `Objective.Intent` exists, is carried, is checkpointed, is in the schema
   and the system prompt. MEASURED 2026-08-31 (design doc, Context): the model
   writes the sentence *after* picking, so it justifies the choice instead of
   constraining the next one — `IntentAge` sits near zero and the sentence is
   rewritten to match whatever gets picked. **It is a caption, not a
   commitment.** Any plan design must assume this failure mode recurs unless
   designed against.

6. **Reasoning is currently unusable, not merely slow.** With thinking on, a
   real menu blows through `maxReplyTokens`, the server reports
   `finish_reason: "length"`, the reply is rejected, and after 3 asks the run
   stops. So today "enable thinking" is not a latency trade — it is a run
   killer unless `POKEPILOT_LLM_MAX_TOKENS` is raised to match.

---

## 4. Recommended architecture

Three tiers. Note that the cheapest tier is not a fast model call — it is
**no call**.

```
  ┌── tier 0 ── EXECUTE ────────────────────────────── 0 model calls
  │   the plan's next step still resolves against
  │   the current menu (agent.Chosen) → run it
  │
  ├── tier 1 ── CHOOSE ─────────────── thinking OFF, ~1s
  │   no plan, or the plan is exhausted, and nothing
  │   material has changed → the loop we have today
  │
  └── tier 2 ── PLAN ──────────────── thinking ON, ~45s
      a replan trigger fired → emit an ordered list of
      objective SENTENCES, then drop back to tier 0
```

Division of labour, stated as a rule:

- **Deterministic code decides what is *possible*.** It already does. Keep
  pushing work down: mart stock, Train's refusal precondition, encounter-band
  feasibility. Every fact moved down here is a model call *and* an emulator
  excursion saved.
- **Non-thinking inference decides *which of these, now*.** Bounded: an index
  into a short list. This is the loop as built, and it is good at it.
- **Thinking inference decides *what we are trying to accomplish over the next
  several rounds*, and only that.** It must produce a plan, never a single
  action — a 45s call that buys one 10s objective is a losing trade; a 45s
  call that buys eight is a winning one, and the trade improves as runs
  lengthen because wasted rounds scale with run length and replans do not.

**Do not add a second model or a second endpoint.** `NoThink` is a request
field. Two `LLMPlanner` values (or one field flipped per call) is the whole
implementation.

---

## 5. Strategic state model

Minimal. Resist the urge to build a hierarchy.

```go
// Plan is what the strategist said to do, in order. Steps are objective
// SENTENCES (Objective.String() form), never menu indices: the menu is
// rebuilt every round, so an index means something different next round.
type Plan struct {
    Goal  string   // the strategist's own one-line statement of purpose
    Steps []string // ordered; consumed front to back
    Step  int      // how far in we are
    Round int      // the round it was made in, for staleness telemetry
}
```

That is the entire structure. Justification for each omission:

- **No preconditions, no postconditions, no "done when".** `Offer` already
  answers "is this legal here?" and `Chosen` already answers "is this step
  still on the menu?". A step's success/failure is exactly `Execute`'s error,
  which is already recorded in `History` and `Knowledge.Failures`.
- **No hierarchy, no sub-plans.** Nothing in the run today needs one, and a
  tree is a thing to keep consistent for no measured benefit.
- **No new persistence format.** `memoryFile` (`agent/memory.go`) already
  carries `Intent`/`IntentAge` beside every checkpoint; add `Plan` the same
  additive way, so an older checkpoint restores with an empty plan — which is
  precisely what a run that had none had.
- **Keep `Intent`.** It is per-choice and it is the S9-7 measurement. The plan
  is a different thing (multi-round, strategist-authored). They coexist;
  do not merge them.

### Who advances the queue

`Run` does, deterministically, and it is the only writer. The fast tier never
edits the plan. See §6 for why.

### Step resolution — the one subtle rule

When a step's sentence does not resolve against the current menu, that is
**not automatically a replan**. `Offer` withholds objectives whose
precondition has lapsed, and lapsing is often *success*: "heal the party at
VIRIDIAN POKECENTER" leaves the menu the moment the party is no longer hurt.

So:

- step does not resolve → **skip it, advance, try the next step**
- every remaining step fails to resolve → the plan is exhausted → tier 1 or
  tier 2 by the trigger table
- step resolves and `Execute` returns an error → count the failure, and
  escalate by the ladder below

---

## 6. Escalation model

The signals the proposal asks for **already exist and are already computed**.
Nothing new needs detecting. What changes is where they route.

| Signal | Where it lives today | Today's effect | Proposed effect |
|---|---|---|---|
| observation unchanged N rounds | `sameProgress` + `stuck`, `run.go:741` | `StopStuck` at 3 | **replan (tier 2)** at 3; stop only if it recurs after a replan |
| N consecutive failures | `consecFailures`, `run.go:702` | `StopFailed` at 3 | **replan** at 2; stop at 4 |
| same objective, same error, twice | `lastFailObj`/`lastFailErr` | `StopFailed` immediately | **replan immediately** |
| retreat streak at unchanged level | `retreatStreak`, `run.go:676` | `StopFailed` at 3 | **replan** at 2 |
| blackout | `skill.ErrBlackedOut` | exempt, continue | **replan** — the world materially changed |
| new wall heard | `Knowledge.HeardRequirement` | prompt text only | **replan** when `Times == 1` (a *new* wall) |
| badge earned / major event set | `Observation.Events`, `Badges` | prompt text only | **replan** — objective completed |
| plan exhausted | — | — | tier 1 if nothing else fired, tier 2 if the last N rounds made no progress |
| reply rejected | `MaxReplyRetries` | stop after 3 | unchanged — this is a model-shape problem, not a world problem |

**The single most valuable change in this whole document is row 1 and row 2:
turn `StopStuck` and `StopFailed` into replan edges.** A run that ended at
round 23 of 200 because it circled three times is a run that never got to ask
a better question. That change is roughly twenty lines in `run.go` and needs
no plan queue at all — see §11 stage 1.

### Retry vs. replan

A single failure retries nothing and replans nothing: it is recorded, the plan
advances, and the next step runs. That is already the behaviour and it is
correct — `Run`'s doc explains why ("blocked at the town exit, hear why, go
get what unblocks it"). Retry lives *below* this layer, inside `skill`
(`GoTo`'s per-leg replanning, `walkWithinMap`'s obstacle retries,
`Travel`'s battle resolution), where it belongs.

### Autonomy of the fast tier

**Zero.** The fast tier is only ever reached when there is no applicable plan
step, so there is nothing to deviate from. This is strictly simpler than a
"fast model may override the plan" rule, and it removes the entire class of
"who won, the plan or the greedy call?" ambiguity from the telemetry. If a
plan is bad, the escalation triggers will fire within two or three rounds and
a strategist call fixes it — that path already exists and needs no second
mechanism.

---

## 7. Example flows

**Normal progression.** Strategist (45s) emits: heal party at Viridian
Center → buy 3 POTION → go to route 2 → go to viridian forest → catch a
PIKACHU here → train the lead to level 12 → go to pewter city → beat the gym
leader here. Rounds 1-8 execute with **zero model calls**. Eight objectives
for one 45s call, versus eight fast calls plus the wandering they invite.

**Pathfinder failure.** Step "go to route 2" returns
`ErrReplanExhausted`. First occurrence: record, advance to the next step. If
the next step also fails, `consecFailures` hits 2 → replan, with the failure
tally and the verbatim wall text in the prompt.

**Stuck.** Steps keep resolving and succeeding but `sameProgress` says nothing
moved (the Route-1-and-back loop). `stuck` hits 3 → replan. The strategist's
prompt carries `Knowledge.Completed` counts, which is exactly the "you have
done this six times" evidence the greedy loop cannot act on.

**Lost gym battle.** `skill.Gym` reports the loss as an outcome; the blackout
fires `ErrBlackedOut`. Blackout is a material change → replan. The strategist
sees a halved wallet, the respawn map, and the lead's level, and can emit
train → heal → challenge rather than walking straight back into Brock.

**Major objective completed.** Boulder Badge appears in `Observation.Badges`.
Badge-set is a replan trigger: the plan that ended at "beat the gym leader
here" is done, and the next plan is a different part of the game.

---

## 8. Inference changes

Small, and mostly already built.

1. **Make `NoThink` per-call.** Today it is a struct field. Either add a
   `think bool` parameter to the internal `ask`, or hold two `LLMPlanner`
   values sharing a URL/model/token. The second is lazier and keeps the
   `Health` counters separable, which is what you want anyway — a strategist
   call and a chooser call should not be averaged together.
2. **`MaxTokens` must differ per tier.** This is not optional: with thinking
   on and `maxReplyTokens = 512`, the reply truncates, `finish_reason` is
   `"length"`, the reply is rejected, and three of those stop the run. The
   strategist needs its own (much larger) cap. `POKEPILOT_LLM_MAX_TOKENS`
   exists but is global.
3. **`Timeout` must differ per tier.** 60s default is marginal for a 47s
   reasoning call.
4. **A second reply schema.** The chooser's schema stays `{choice, intent}` —
   do not touch it; `choiceSchema`'s comment records three separate runs
   killed by a model stapling optional fields to the wrong choice. The
   strategist gets its own: `{goal: string, steps: [string]}`, with steps
   constrained to a `maxItems` (8-12 is the right order of magnitude).
   Steps are free text validated by `Chosen` against the menu, not by the
   schema.
5. **Sentence resolution already exists.** `Chosen` accepts an
   `Objective.String()` form, case-insensitively, with no fuzzy matching. A
   strategist step that does not match exactly is skipped, not guessed at.
   That is the right failure mode and it is already the code's rule.
6. **The strategist prompt must not become a walkthrough.** Everything in
   `badgerun`'s `-inject-fact` discipline applies: the strategist gets the
   Goal and the observation, never the answer.

One warning, from item 11: **this change voids every existing scoreboard
row.** The prompt-hash idea in that item should land alongside this work, or
the comparison will be lost the same way it has been lost three times before.

---

## 9. Observability

`farm.LLMStats` + `cmd/pokepilot/stats.go` is already the right seam and
already carries `Calls`, `Rounds`, `Rejected`, `Repeats`, `AvgOffered`,
`LastSeconds`, `AvgSeconds`, `PromptTokens`, `CompletionTokens`, `Transport`,
`Fallbacks`, `Intent`, `IntentAge`, `Choices`.

Add the minimum that answers the two questions asked:

| Field | Answers |
|---|---|
| `StrategicCalls`, `FastCalls` | are we invoking reasoning too often? |
| `ThinkSeconds`, `PlaySeconds` | time reasoning vs. time playing |
| `ReplanReasons map[string]int` | *why* reasoning was invoked, by trigger name |
| `PlanSteps`, `PlanStep`, `StepsExecuted` | how far a plan gets before it dies |
| `StepsPerStrategicCall` | **the headline: useful progress per reasoning call** |
| `StepsSkipped` | steps that stopped resolving — plan staleness |

`Repeats` stays the wander signal and becomes the before/after number for the
whole change: if the plan queue works, `Repeats` falls.

Nothing else. Do not build an event bus. The run log line, `prompts.txt`, the
checkpoint ring and `LLMStats` are four surfaces already, which is plenty.

### Training data

The record wanted —

```
game state + objective + options + decision + intents + outcomes + result
```

— is nearly complete today: `prompts.txt` holds every prompt verbatim,
`run.log` holds every choice and outcome, `checkpoints/` holds the exact state
each decision was made in plus the knowledge at that moment, and
`FinishReport` holds the run's terminal verdict. What is missing is the *plan*
and its *fate*. Persisting `Plan` into `memoryFile` supplies both, and it is
one additive field. That is the extent of what should be done for training
now.

---

## 10. Tradeoffs, and where this design could be wrong

**The queue may be the wrong abstraction if the model cannot write good plans.**
This is the real risk, and the closest evidence we have is discouraging:
`Objective.Intent` was built as exactly this kind of slot, and it is measured
to be rubber-stamped — written after the pick, rewritten every round. A plan
asked for at replan time is structurally different (it is emitted *before* the
picks it governs, which is the whole point), but the design doc's open
question 4 is right that the rubber-stamp failure should be assumed to recur
if the strategist is shown its own previous plan. The gating number is the
design doc's open question 1: **how many steps does a plan survive?** If the
answer is 3, the 45s call is a tax.

**A cheaper alternative that should be tried first.** Escalation without a
queue: keep the greedy loop, but when `stuck`/`consecFailures` fires, make one
thinking-enabled call whose answer is still a single menu index, then continue
greedy. This is stage 1 in §11. It captures most of the value (the run stops
dying at round 23) at a fraction of the complexity, and it produces the
measurement that says whether the queue is worth building.

**A queue-free alternative that might beat both.** Nothing here says the
strategist's output must be a queue. It could instead be a *constraint* the
chooser is given each round: one sentence at the top of the fast prompt
("you are working toward the Boulder Badge; you have already trained twice
without healing"). That keeps a model call per round (~1s, cheap) and drops
the entire stale-plan problem. It is worse on latency and better on
robustness. If plan staleness turns out to bite, this is the fallback.

**The menu is the cheaper lever, and it is unexploited.** Latency is measured
to scale with menu size (6-7s at 5-7 offered, 21-23s at 13-15). Item 17
proposes collapsing the combinatorial entries. Shrinking the menu speeds up
*every* call in every tier, has no strategic risk, and the fix pattern already
shipped once (S9-1's flee argument). If the goal is "play faster", this may
have a better ratio than anything in this document.

**Honest cost of the plan queue.** It puts a second author of behaviour into
the loop. Today exactly one thing decides what happens — the model, from a
menu. With a queue, `Run` decides too (skip, advance, replan). That is a real
increase in the thing this project has been careful about, and the mitigation
is that every one of `Run`'s decisions is mechanical and observable: does the
sentence resolve, did it error, has the observation changed.

---

## 11. Incremental implementation path

Each stage is independently useful and independently measurable. Stop at any
of them.

**Stage 0 — free wins, no architecture (do these regardless).**
Land items 1 and 20 from `SLICE10-CANDIDATES.md`: don't offer Train to a lead
that will refuse it; read the mart's shelf instead of assuming POTION. Each
removes a guaranteed-wasted round, which is a model call plus an emulator
excursion. Also land item 11's prompt hash, because everything after this
point changes the prompt.

**Stage 1 — escalate instead of stop. ~20 lines, no new types.**
In `run.go`, at the points that currently set `StopStuck` and `StopFailed`:
make one thinking-enabled call (a second `LLMPlanner` with `NoThink=false`, a
raised `MaxTokens` and `Timeout`) asking the same menu question, and continue.
Stop only if the same trigger fires again after the escalation. Record
`ReplanReasons` and `StrategicCalls`.

*Measured by:* does a run get past round 23? `Repeats` before/after.

**Stage 2 — measure plan lifetime by hand, before writing strategist code.**
The design doc's open question 1, already filed as an idea: take the measured
23-round run and ask how many steps a strategist could have committed to at
round 1 given only what was observable then. Run the thinking-off plan
ablation in the same pass (open question 2). Costs an afternoon and no code,
and it gates the whole of stage 3.

**Stage 3 — the plan queue.**
`Plan` struct, plan-shaped strategist schema, `Chosen`-based step resolution,
skip-on-unresolvable, replan on the §6 trigger table. Persist into
`memoryFile` additively. Tier 0 executes with zero model calls.

*Measured by:* `StepsPerStrategicCall`, `ThinkSeconds` vs. `PlaySeconds`,
`StepsSkipped`, and `Repeats`.

**Stage 4 — shrink the menu (item 17).**
Independent of everything above and it speeds up every tier. Deliberately
last because S9-11/S9-12 should size it first, and because it trades a
numbered choice for a reply argument, which is the harder question for a small
model.

---

## Appendix — the 17 questions, answered directly

1. **Where do we call the LLM unnecessarily?** Nowhere by *count* — one call
   per round is already minimal. But ~7 of 23 rounds in the best measured run
   produced no progress, so ~30% of calls were wasted by *content*, not
   frequency. Plus the guaranteed-failed rounds from `Offer` (items 1, 20).
2. **What genuinely benefits from reasoning?** Sequencing several objectives
   toward a goal; recovering from a blackout or a stated wall; deciding
   whether the party is ready for a gym; noticing "I have done this six
   times". All multi-round, all currently answered one round at a time.
3. **What should use thinking disabled?** Picking one index when a plan is
   exhausted and nothing has gone wrong. That is the existing loop.
4. **What needs no LLM at all?** Executing a plan step that still resolves
   (tier 0). Navigation, battle turns, menu operation, dialogue paging,
   fight/flee mechanics, mart stock, train feasibility — all already
   deterministic or should be.
5. **Persistent plan/intent queue?** Yes, but only after stage 2's plan-
   lifetime measurement. Stage 1 delivers most of the value without one.
6. **What should an intent contain?** An objective sentence in
   `Objective.String()` form. Nothing else — no preconditions, no timeouts,
   no ids.
7. **Who advances the queue?** `Run`, mechanically, as the sole writer. Never
   the fast tier.
8. **Success/failure of an intent?** `Execute`'s error, exactly as today.
   Resolution failure against the menu is a *skip*, not a failure.
9. **Retry vs. replan?** Retry lives inside `skill` (per-leg replanning,
   obstacle retries). At the agent layer a single failure advances the plan;
   two consecutive, or the same failure twice, replans.
10. **Detecting stuck without false positives?** Do not build anything —
    `sameProgress`, `stuck`, `consecFailures`, the identical-failure guard,
    `retreatStreak`, `Requirement.Times` and `Knowledge.Completed` counts all
    exist and are already tuned by measured runs. Rewire them, don't rewrite
    them. (Item 16 flags that `defaultStuckAfter = 3` now carries weight it
    was not sized for — worth one measurement.)
11. **How much fast-tier autonomy to deviate?** None. The fast tier only runs
    when there is no applicable plan step.
12. **When is a plan invalidated?** Blackout; badge or major event set; a
    newly-heard wall; two consecutive step failures; the same failure twice;
    stuck for 3 rounds; plan exhausted.
13. **Fixed queue, hierarchy, or conditions?** Flat ordered list of
    sentences. A hierarchy is unjustified; conditions are already expressed
    by `Offer` deciding what is legal.
14. **Preventing stale plans?** Re-resolve every step against the freshly
    built menu (`Chosen`). Unresolvable → skip. That is the staleness check,
    and it costs nothing because the menu is rebuilt every round anyway.
15. **Can the same model switch thinking per request?** Yes, already —
    `chat_template_kwargs: {"enable_thinking": false}`, measured on this
    server. `MaxTokens` and `Timeout` must move with it, or the thinking call
    truncates and is rejected.
16. **What does the strategic call need that routine calls do not?** The
    non-scrolling evidence: full `Knowledge.Completed` counts, the whole
    failure tally, every wall heard with its `Times`, the badge/event list,
    money and respawn point, and rounds remaining. The chooser can survive on
    the current observation; the strategist cannot.
17. **Can better deterministic understanding reduce strategic calls?** Yes,
    and it is the cheapest lever available: mart stock, train feasibility,
    encounter-band vs. lead level, and the graph gaps in `GRAPH-GAPS.md`.
    Every wall the code can predict is a replan the model never has to pay
    for.
