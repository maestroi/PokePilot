# Slice 10 candidates — raised 2026-08-30

Raised against `eba69eb`, the head of `agent-plan/0f08e43a-8a98-4ff4-8bd4-052c70f564f2`
— **slice 9's branch, which is NOT merged to main.** Unlike the slice 9 doc,
much of what follows IS blocked on that branch: items 1, 2, 3, 5 and 16 name
code that exists only there. Items 11-15 and 17 read on main as well.
Every line below says which base it was read on.

Labelling, as always: MEASURED means someone ran it, DERIVED means it was read
out of the tree, UNMEASURED means it needs a task before it can be scoped.

---

## PROVISIONAL: slice 9 is still running

At the time of writing, plan `0f08e43a` has 8 of 11 tasks done. Three are open:

| # | Task | State |
|---|------|-------|
| 9  | S9-10: does the generalised Gym actually beat Misty? | running |
| 10 | S9-11: what does the menu cost now — offered size against latency | pending |
| 11 | S9-12: the milestone — how far does a long goal get, and what stopped it | pending |

**S9-12 was designed to be this document's input.** Slice 9's own sequencing
says so: "Measure a long goal-driven run. How far does it get, and what
stopped it. The answer is the input to slice 10, and 'it stopped at a missing
verb' is a successful measurement."

So this doc is written from the survey and the runnotes, not from the
milestone. When S9-12 lands, **amend this file before creating the plan**, and
expect its answer to reorder the sequencing — it may promote a reach item
(6-10) above the consolidation spine, or it may name a stopper nobody here
anticipated. Do not treat the ordering below as settled until then.

---

## The proposed goal: stop paying interest, then open the road

Slice 9 shipped nine things and deferred a named list of follow-ups behind
them, each one written down in RUNNOTES.md by the task that chose to defer it.
That list is not vague — it is specific, measured, and every item names its
own file. Meanwhile S9-3's survey turned the Elite Four from a wish into a
sized route with three concrete blockers.

Slice 10 pays the interest first and then opens the road. The consolidation
items are small, they are all in code slice 9 just wrote (so the context is
warm), and two of them are actively costing measured run time right now.

---

## 1. A hurt lead is still offered Train, and Train now refuses

**DERIVED** — `agent/offer.go:338` on `eba69eb`:

    if obs.HasGrass && len(obs.Party) > 0 {
        if target := int(obs.Party[0].Level) + trainStep; target <= 100 {
            out = append(out, Objective{Kind: KindTrain, Level: uint8(target)})
        }
    }

The gate is grass plus a party. It has never heard of HP.

S9-9 added a retreat line at half max HP, checked **at session start** as well
as after every battle (`skill/train.go`, `retreatLineNum/Den`). So a lead below
half HP standing in grass is offered Train, picks it, and `skill.Train` refuses
before it fights anything. That is a round that spends a planner call, a
frame budget slice and a failure slot to change nothing.

S9-9's runnote states the gap plainly: "A planner that sees `stopped while the
party was alive` should walk to a center (or use an item) before re-offering
Train; nothing enforces that yet, and StuckAfter is the only backstop."

**The fix is in-pattern, and the pattern is already three deep.** `Offer`
already withholds an objective that cannot change anything: a satisfied
starter, a full party offered the travelling heal (`agent/offer.go:291`), a
potion offered to an undamaged party (`:301-308`), a gym whose badge is already
held (`:327`). "Do not offer Train to a lead below the retreat line" is the
same rule, one condition, and the retreat line is already a named constant.

**MEASURED already, by S9-9:** three grind cycles from `post_errand` put the
lead at 18.5%, 12.9% and 2.6% of max HP in the battles before it fainted. The
lead spends real time below the line. This is not a rare state.

Note what this does NOT need: no new verb, no planner change, no prompt text.
`Observation.Party` already carries HP and MaxHP (`agent/observe.go:167-171`),
and `partyHurt`/`monHurt` already exist in the same file.

---

## 2. Train's leg budget is a fixed number against a random process

**MEASURED 2026-08-30 by S9-8** — the sharpest single number slice 9 produced:

> P(≤2 encounters in 141 legs at 8/256) ≈ **18%**; and maxLegs=140 sits only
> ~⅕σ above the mean legs for 4 battles (128±66), so **~35% of sessions trip
> the budget at all** (P(≤3)≈0.35).

**DERIVED** — `skill/train.go:162` on `eba69eb`:

    maxLegs := 20*maxBattles + 60

A constant, sized once, against a per-step encounter roll of ~3% on a typical
map. S9-8 refuted the phase-lock hypothesis and established the real mechanism:
`hRandomAdd` advances by rDIV every frame, a ping-pong grind samples that fixed
sequence at a constant stride, and some 141-leg windows of the sequence simply
contain almost no values under the rate. It is a drought window, not a lock.

**Roughly one training session in three dies on this**, and the tests' current
answer is `StepFrames(123)` — resample the window and hope. S9-8's runnote
labels that a workaround in as many words and hands the principled fix to a
later task: "budget legs by expected rolls (want ÷ rate/256, with slack for the
heavy tail) instead of the fixed 20×maxBattles+60, or make Train vary its
stride so a drought window cannot persist."

The two named options are different in kind and the task should pick
deliberately: budgeting by expected rolls makes the failure rarer but keeps the
sampling degenerate; varying the stride attacks the degeneracy itself. **The
rate and the species count are already read** — `NoEncounterDiagnostic` takes
both — so the inputs to an expected-rolls budget are in hand at the call site.

This is the item with the best measured return in the whole document: it is
one expression, and it buys back a third of the training sessions.

---

## 3. `Budget.ResumeFrom` has no callers

**MEASURED 2026-08-30**, by grep on `eba69eb`:

    $ git grep -n "ResumeFrom" -- cmd/ farm/
    (no matches)

S9-5 built resume: `Budget.ResumeFrom` takes a checkpoint `.state` path, loads
it, and loads the paired `.knowledge-v1.json` from the same base name before
round 1. It is tested (`TestRunResumesKnowledge`) and it works. Nothing calls
it. `cmd/badgerun/main.go` is the only thing outside `agent/` that even knows
`CheckpointDir` exists.

**This is the third instance of the same shape.** Slice 7 found it (affordances
built, not wired), slice 9's item 0 found it again (`skill.Flee`,
`skill.TravelFlee`, `skill.UseFieldItem` unreachable from any planner), and
here it is a third time. Worth stating as a project fact rather than a
recurring surprise: **a slice that builds a seam should wire a caller in the
same slice, or the next slice pays to rediscover it.**

The seam is small: the newest `.state` in the run's `checkpoints/` dir is the
lexicographic max (frame numbers in the names sort correctly). S9-5's runnote
names one wrinkle to decide rather than discover: resumed round numbers restart
at 1, and a resumed run writing into the same ring leaves old entries until
they evict. If the wall's tooling reads round numbers, offset the counter.

Why it matters beyond tidiness: slice 9's item 3 argued that for a goal
measured in hundreds of rounds, a crash without resume is a total loss. The
machinery to prevent that exists and is switched off.

---

## 4. badgerun still cannot tell a loop problem from a capacity problem

**MEASURED 2026-08-30**, by grep on `eba69eb`:

    $ git grep -n "Health\|ReplyRetries" -- cmd/
    (no matches)

S7-3 built `LLMPlanner.Health{Transport, Rejected, Fallbacks}` and asked S7-4
to print it on each scoreboard row. S7-4 built `Result.ReplyRetries` and asked
the next task for the same thing. Both runnotes say it in their "for the next
task" section. Three slices later, `cmd/badgerun`'s table has neither column.

S7-3's runnote states why it matters: the health counters are "what makes
ablation A's 'not capacity' reading trustworthy", and S7-4's calls the pair
"the diagnostic that separates a loop problem from a capacity problem".

The scoreboard is the project's instrument. Reading a row today, a run that
died on transport timeouts and a run that died on a model that cannot follow
the schema look identical. Two struct fields and two columns.

Small enough to ride along with any other task in this slice.

---

## 5. Three loose threads slice 9 left, each one line

**DERIVED** on `eba69eb`:

- **`skill/zz_train_dynamics_test.go` is committed scratch.** Left by `5f379dd`,
  flagged for deletion by S9-8's runnote, still tracked. It is the only `zz_*`
  file in the tree (`git ls-tree -r --name-only` confirms). Every other task
  deleted its scratch; this one escaped.
- **`maxHealDetours = 6`** (`skill/gym_test.go:38`, used by `gym_test`,
  `journey_test` ×2, `travel_test`) was sized for the pre-retreat damage
  cadence. S9-9's runnote: the retreat line makes heal detours more frequent,
  so a tight grind may now trip the cap — "if TestGymBoulderBadge or a journey
  test dies on 'N heal detour(s)', **raise the constant — do not lower the
  line**." Whether it actually trips is UNMEASURED. Worth one deliberate look
  rather than a surprise at 3am in a wall run.
- **The live requirement-harvest test is still pending.** S9-6's harvesting is
  proven by unit tests against real ROM text, but no live run has driven a Talk
  through the tape into `Observation.Requirements`. S9-6's runnote explains
  exactly why (the reachable requirement lines sit behind S8-6/S8-7's forest
  stalemate and the Route 2 fragmentation, and the near wall line's sprite is
  not rendered in the `post_errand` fixture) and says the test is "a few lines
  on top of the existing capturePlanner harness" once a line is reachable.
  **Item 12 is the cheap way to make it reachable** — a fixture standing at the
  Viridian Forest north gate puts the Super Nerd's "You need to look
  everywhere..." one Talk away, without adopting anyone else's slice and
  without waiting for item 6's graph work. Sequence it after 12, not after 6.
  And heed the runnote's closing warning: do not add requirement shapes to
  force a match with an unreachable line; that is overfitting the filter to a
  test.

---

## 6. The graph cannot route a road the agent has already walked

**DERIVED** — `docs/ROAD-TO-ELITE-FOUR.md` (S9-3, commit `d8d70ee`), the
sizing document for everything from here on. Cerulean → Indigo is 8 map legs.
Legs 1-4 and 6 are verified on-foot paths that `Travel`/`GoTo` execute today.
Three are not, and **two of the three are graph problems, not verb problems**:

- **Leg 5, Route 2 → Viridian: a phantom edge.** The map header declares the
  connection; row 22 of Route 2 is solid across all 20 columns (metatiles
  `$50`/`$3D`, not in the OVERWORLD walkable list), and BFS confirms neither
  half reaches the other on foot. The real crossing is Viridian Forest via two
  gate buildings — a measured 130-step path from `(2,1)` to the south warp
  `(16,47)`. No badge, no HM, no script check: pure geometry. **The agent has
  already made this crossing in a live run.** It is invisible to a map-level
  graph that treats `0x0D` as one node.
- **Leg 7, Route 22 → Route 23: a phantom edge plus a dead-end building.**
  Route 22's north rows 0-1 are solid; the only exit is gate `0xC1`, whose
  warps all point at `0xFF` ("the map you came from"), so `world.BuildGraph`
  resolves it as a dead end and the planner cannot route *through* it. Also
  needs the Boulder badge (`pokered/scripts/Route22Gate.asm:63-64`), which is
  on the road.

**This is the same finding as S8-9's collision-grid fragmentation**, which
already measured Route 2, Route 4, the forest and Mt. Moon as fragmented. One
fix — a graph whose nodes are connected components rather than whole maps —
plausibly serves both, and that shared root is worth establishing before either
is patched separately. UNMEASURED whether it does; that is the first question
of the task, not an assumption to build on.

**The task's OTHER opening question, and it is cheap: seven maps nobody has
explained.** The survey reports `world.BuildGraph` parsing 228 maps with 221
reachable from Cerulean, and lists the unreachable as `0B 45 4B 4E AD E7 EF F0`.
`0x0B` is accounted for — the survey establishes it as dead data (invalid header,
block address below `0x4000`). The other **seven have no explanation anywhere in
the tree**. And `0xC1`'s failure mode is precisely a mechanism that makes a map
resolve as unreachable: every warp pointing at `0xFF`, the "map you came from"
sentinel, which a graph builder cannot resolve without knowing the approach.

If those seven are the same bug, this item's fix reaches considerably further
than the two legs it was scoped for, and the slice should know that before it
is sized rather than after. If they are seven separate quirks, that is worth
knowing too — it is the difference between one fix and a list. Either way it is
a scripted read of seven map headers, not a slice.

Milestone shape: the Cascade Badge, or Viridian reached by a route the graph
planned rather than one the planner stumbled into.

---

## 7. Surf, and the acquisition chain nobody has surveyed

**DERIVED** — `ROAD-TO-ELITE-FOUR.md`, G4. Leg 8 (Route 23 → Indigo) is 20×144
tiles with **rows 81, 92 and 101 full-width non-walkable**: water `$14` and
cliff tiles, and PLATEAU's walkable list is only `$1b $23 $2c $2d $3b $45`
(`pokered/data/tilesets/collision_tile_ids.asm:69-70`, byte-identical in the
ROM). PLATEAU is a water tileset and the ROM's Surf check accepts `$14` plus
shore tiles, so **Surf crosses these bands**. Once you can swim, the rest is
plain walking — a measured 26-step path from `(9,1)` to row 20.

The verb does not exist. `KindUseItem` uses a bag item on a *party member*;
there is nothing for using an HM on the world. It is the survey's S10-1 and its
"biggest wall".

The acquisition chain is known and needs no new verb of its own: GOLD_TEETH is
a field pickup in Safari Zone West (`pokered/scripts/SafariZoneWest.asm:9`),
and the Fuchsia Warden trades it for HM04 (`pokered/scripts/WardensHouse.asm:14,47`);
the Warden's House `0x9B` is reachable only from Fuchsia `0x07` at `(27,27)`.
`KindPickup` and `KindTalk` already cover both halves.

**The open question that makes this a bad slice to open on**, stated by the
survey itself: the route Cerulean → Fuchsia was NOT surveyed, and whether it
needs Surf is unknown. If it does, the chain is circular and the real entry
point is somewhere else entirely. **A survey task, deliverable a document, must
come before any Surf code** — the same shape as S9-3, and for the same reason
both Phase 1 failures happened: wrong assumptions in a spec, not bad code.

---

## 8. Five off-road gyms, and what each detour costs

**DERIVED** — leg 8 needs all eight badges, checked by seven sprites on Route
23 (`pokered/scripts/Route23.asm:153-193`). Two are on the road (Boulder from
Pewter, Cascade from Cerulean, where the run already stands). Five are detours:
Vermilion/Thunder, Celadon/Rainbow, Fuchsia/Soul, Saffron/Marsh,
Cinnabar/Volcano.

`KindGym` was generalised in `0c6c9b4` to read a gym table instead of
hardcoding Brock — **and S9-10 is testing right now whether it actually beats
Misty.** That result lands before this slice is planned and should be read
before this item is scoped: if the generalised Gym does not beat the second
leader it meets, "five more gyms" is a very different task.

Two UNMEASURED dependencies the survey flags and refuses to guess at: Cinnabar
may itself need Surf, and Saffron's gym may need Flash for its darkness (HM05
is given unconditionally by Oak's aide in Route 2 Gate `0x31`,
`pokered/scripts/Route2Gate.asm:11,27` — on the road, and free). Verify, do not
assume.

The survey sizes this as three slices (S10-3/4/5) that "could merge into one
badge tour if each gym proves to be a short walk from the last". That
conditional is the measurement, and it is what item 6's routing work makes
answerable.

---

## 9. The Elite Four is a battle nothing can start

**DERIVED** — `ROAD-TO-ELITE-FOUR.md`, G5. `pokered/scripts/IndigoPlateau.asm`
is five lines of text pointers: no `CheckEvent`, no badge check, no guard
sprites. The door is not gated at all. The entrance warps `(9,5)`/`(10,5)` lead
to the lobby `0xAE`, which chains `0xF5` → `0xF6`.

The final gate is the League battle itself, and the objective vocabulary has no
verb for it. `KindGym` fights "the leader of whichever gym the player is in";
there is no verb for the League's four-plus-one sequence, and none for any
non-gym trainer either — which is the more general gap and probably the one
worth building.

Last item in the survey's ordering and last here. Named so the slice knows what
it is walking toward, not because it belongs in slice 10.

---

## 10. Does a plan survive a run that is longer than its memory?

Slice 9 built the plan slot: `Objective.Intent` (200 bytes, typed rejection
over cap), `Observation.Intent`/`IntentAge`, carried by `Run`, persisted beside
each checkpoint by S9-5, and declared in the schema and the system prompt. Run
never writes, edits or summarises the sentence — deliberately, because
synthesising it would be planning for the model again.

**Whether the model uses it is unmeasured, and S9-12 is the measurement.**
S9-4's runnote pins the reading in advance, which is the right way round: "If
the model ignores the intent field live (empty on purpose), that is a FINDING
to write down — do not make Run synthesise an intent; that defeats S9-7's
measurement."

So this item has no task yet on purpose. After S9-12 it becomes one of:

- the model keeps a coherent intent across rounds → the slot works, and the
  question moves to whether intent should influence what `Offer` offers;
- the model rewrites it every round → it is being treated as scratch, not
  memory, and the prompt is what needs work;
- the model leaves it empty → the finding is that a 4B model will not volunteer
  structure, and the honest next move is an ablation against a larger one
  (`POKEPILOT_LLM_MODEL` + `POKEPILOT_LLM_URL`, which is all ablation A ever
  needed).

**Write the reading down before the run, not after.** All three outcomes are
informative; none of them is a failure.

---

## 11. Every slice voids the scoreboard, and the warning is prose

**MEASURED**, by reading three runnotes that each say the same thing:

- S7-2: "every prior badgerun row was scored WITHOUT a goal; new rows carry
  one. They are not comparable — say so when reading them side by side."
- S9-4: "every prior badgerun row was scored without the intent sentence in the
  prompt and without Intent/IntentAge in the observation JSON (~20 extra
  tokens/round). Not comparable side by side."
- S6-12 voided the entire board for four separate reasons.

Three times, the project has protected comparability with a sentence in a file
that nobody re-reads at the moment of comparison — which is months later, by
someone holding two tables. That is not a process failure by any of those
tasks; each did the right thing available to it. It is a missing mechanism.

**DERIVED**: the prompt is fully determined by four things in the tree —
`llmSystemPrompt`, `LLMPlanner.ExtraSystem`, `LLMPlanner.Goal`, and
`choiceSchema` (`agent/llm.go`). Hash them together, print the short hash on
every badgerun row and at the head of each run's `prompts.txt`.

Then two rows from different prompt generations cannot be silently compared:
the difference is a visible column, not a remembered caveat. Cheap, mechanical,
and it protects every future slice — each of which will change the prompt
again, because that is what these slices do.

Note what it is NOT: not prompt versioning, not a migration, not a registry.
A hash of the bytes actually sent, stamped where the numbers are read.

---

## 12. The fixtures stop at Viridian, so every road test walks the road again

**DERIVED** — the full registered set (`skill/fixture/fixture.go`, `init`):
`reds_bedroom`, `post_starter`, `pallet_town`, `viridian_city`,
`viridian_pokecenter`, `viridian_mart`, `post_errand`, `post_pokeballs`.

**Nothing past Viridian.** The agent reaches Cerulean. So every Pewter, forest,
Route 2/3/4 and gym test starts at `post_errand` and re-drives the whole road
through the emulator, every run.

**MEASURED**, the cost, from three different runnotes:

    fixture cache, cold (empty dir)      43.2s   (S7-1)
    fixture cache, warm (reused)          1.7s   (S7-1)
    TestBattleAnswersForgetMovePrompt      269s   (S9-9)
    TestTrainSurvivesEvolution             170s   (S9-9)
    TestGymBoulderBadge, Pallet->Brock     ~87s of emulation  (slice 9 item 6)
    go test ./agent                    95-117s
    go test ./skill                       107s

S7-1 already proved the mechanism pays for itself by a factor of 25. The
fixtures simply stop short of where the agent now lives.

`post_pewter`, `post_forest` and `post_cerulean` (names indicative) cut the
long tail of that table. **And a `post_forest` fixture at the north gate is the
cheap unblock for item 5's live requirement test** — the Super Nerd's "You need
to look everywhere..." is one Talk from that tile, no graph work required.

One caution from the same runnote: fixtures are built by driving the game, so a
fixture that far along takes real time to build cold and its builder is a
program that must keep working. Set `POKEPILOT_FIXTURE_DIR` to a shared path
when running the suite — clean worktrees no longer rebuild from boot.

---

## 13. `StopDone` is the zero value, and that has already cost a day

**DERIVED** — `agent/run.go:23`:

    StopDone   Stop = iota // the planner reported ErrDone

So `StopDone == Stop(0)`, and "nobody set a stop reason" is indistinguishable
from "the planner finished". S7-4's first verification attempt FAILED on
exactly this: `planWithRetries` returned StopDone, `Run` checked `stop != 0`, a
finished planner read as "keep going", `Execute` ran on an empty objective
(`KindGoTo` is also `Kind(0)`, so it rendered as a bare "go to"), and five
fixture tests died with StopFailed.

The fix at the time was correct and local — break on the error, never on the
stop value — and the runnote warns the next task by hand: "Any new Stop reason
must NOT be appended after StopDone: StopDone is Stop(0) and every 'is this a
stop?' check in Run breaks on the error, not on the value."

That warning is load-bearing and it is a comment. `StopUnset Stop = iota` ahead
of `StopDone` makes the zero value mean what it should, and the trap stops
being something each future task has to be told about. Small, and it touches
the stop-value comparisons — which is the point: the compiler finds them.

---

## 14. The token bill is logged every round and never added up

**DERIVED** — `agent/llm.go:167-173`: every call already logs
`tokens %d prompt/%d completion` when the server returns a `usage` block, and
`chatUsage` is parsed into `chatResult.Usage` (`:412-413`, `:449`, `:532`).

`Result` has no total. `cmd/badgerun`'s table has no column. The number flows
past once per round and is dropped.

Slice 9's item 6 named cost a design constraint in as many words: the menu grew
this week, a longer menu is a slower round, and a goal measured in hundreds of
rounds multiplies both. The instrument for that argument is already 90% built.

Rides along with item 4 — health counters, reply retries and token totals are
one small task on `runResult` and `formatTable`, not three.

---

## 15. Nothing branches two arms from one save state

**DERIVED** — `docs/AGENT.md` states the rule plainly, and has since slice 4:

> A seed is for run diversity, not for a fair comparison: two policies under
> different seeds have different luck, so a difference in outcome is not
> evidence. **Branch both arms from one save state for that.**

`cmd/badgerun` varies starter and seed. That is the comparison the same
document says is not evidence. Nothing in the tree branches two arms from a
common state.

The reason it was never built is that it needed a save state mid-run to branch
from — which is exactly what S6-11's checkpoint ring and S9-5's `ResumeFrom`
now provide. **Item 3 wires the resume seam; this is the thing worth wiring it
for.** Two runs from one checkpoint, differing in one variable, is the only
honest A/B this project has ever been able to describe.

Sequence it after item 3, and expect it to be small once resume has a caller.

---

## 16. Two exemptions now rest on a constant sized before either existed

**DERIVED** — `agent/run.go:95`: `defaultStuckAfter = 3`, with the comment
"Small on purpose: a run that is not stuck will change the map, the position,
the party, or the events within a few rounds."

Since then, two failure kinds have been exempted from the consecutive-failure
budget and explicitly handed to StuckAfter as their backstop — blackout
(`d815ea2`) and train retreat (S9-9). `run.go:287` says so: "a planner that
ignores it trips StuckAfter, because a retried train leaves the player where it
started."

That last clause is the worry. **A retreat leaves the observation unchanged by
construction** — same map, same position, party HP barely moved. It trips the
stuck counter fast. Whether 3 is right for a planner doing the *correct* thing
(retreat, walk to a center, come back) is UNMEASURED in both directions: too
low ends legitimate recoveries, too high lets a doomed loop burn the budget.

One measurement, and either a new number with a reason or a note saying 3
survived scrutiny. Both are useful; the current state — a constant carrying
weight it was not sized for — is not.

---

## 17. The menu is combinatorial, and S9-1 already shipped the fix pattern

**MEASURED 2026-08-29**: an LLM call takes ~6-7s with 5-7 objectives offered
and ~21-23s with 13-15. **S9-11 is pending and will measure the current size** —
read it before scoping this.

**DERIVED**, what generates the menu today (`agent/offer.go`):

- one `KindCatch` per wild species in the map's table (`:282-288`) — up to 5;
- one `KindUseItem` per (field medicine × party slot it would affect)
  (`:301-322`) — a product, not a sum;
- one `KindGoTo` per known place — 20+ and growing with every slice that names
  more places;
- plus starter, gym, train, heal, buy, pickup.

**S9-1 solved this exact problem once already and the reasoning is on the
record**: it refused to add a "go to X, fleeing" entry per place because
"doubling the menu spends the run's whole latency budget to express one
boolean", and used a reply ARGUMENT instead — `ReplyArgs.Flee`, range-checked
by `WithArgs`, declared in `choiceSchema`.

That template applies twice more, to the two products above: one `Catch` with a
species argument, one `UseItem` with an item and a target argument. The
argument path, its validation and its typed rejections are all built and tested.

Worth stating the tension honestly: an argument is harder for a small model
than a numbered choice, which is why the menu was flat to begin with. S9-1's
live behaviour under `flee` is the evidence for whether the pattern generalises
— so read S9-11 AND S9-12 before committing to this one.

---

## Suggested sequencing

The spine is consolidation — small items, in code slice 9 just wrote, several
costing measured run time today. The reach items follow, gated on the survey
and on S9-12.

**A. Stop the bleeding (two tasks, both one expression each)**

1. **Don't offer Train to a lead that will refuse it (1).** One condition, in a
   file with three precedents for it. Removes a guaranteed-wasted round.
2. **Budget Train's legs against the roll, not a constant (2).** ~35% of
   training sessions currently trip a fixed number. Best measured return in the
   document.

**B. Make the instrument trustworthy (one task, four small parts)**

3. **The scoreboard task (4, 14, 11) — health counters, reply retries, token
   totals, prompt hash.** All four are columns on the same table, reading
   values that already exist and are already dropped. Two of them were asked
   for by name in a runnote three slices ago. Do them as one task, not four.
4. **Wire `ResumeFrom` (3).** A built, tested seam with no callers — the third
   time this slice-shape has appeared.
5. **A/B from one save state (15).** The comparison `docs/AGENT.md` has
   described since slice 4 and nothing has ever performed. Small once 4 lands,
   and it is what makes every later measurement in this slice mean something.

**C. Make the tests cheap, which makes everything after it cheap**

6. **Fixtures past Viridian (12).** Cuts the measured long tail (269s, 170s,
   87s per gym journey) and unblocks the next item for free.
7. **The live requirement test (5, third bullet).** A few lines on the existing
   `capturePlanner` harness once a `post_forest` fixture exists. **Moved ahead
   of the graph work** — item 12 reaches the Super Nerd's line more cheaply
   than item 6 does.
8. **Sweep the loose threads (5) and close the zero-value trap (13).** Delete
   the tracked scratch test, look at `maxHealDetours` deliberately, add
   `StopUnset` so the compiler enforces what a runnote currently enforces by
   asking nicely.

**D. The slice's real work**

9. **One graph for a fragmented world (6).** Open with its two cheap questions —
   do the phantom edges and S8-9's fragmentation share a root, and are the seven
   unexplained unreachable maps the `0xFF`-warp bug — because both answers
   resize the task. Then legs 5 and 7. This is what turns the road from "walked
   by luck" into "planned".

**E. Sized off measurements that have not landed yet**

10. **StuckAfter against its two new exemptions (16).** One measurement; either
    a new number with a reason, or 3 survived scrutiny.
11. **Menu shaping (17), read against S9-11 and S9-12.** The argument pattern
    from S9-1, applied to Catch's species and UseItem's target — but only if
    the live evidence says a small model handles arguments well enough.
12. **Survey Cerulean → Fuchsia (7).** A document, not code, exactly like S9-3.
    It says whether the Surf chain is circular, and it sizes 7 and 8 both.
13. **Then, sized off that survey and off S9-10's result:** the Surf verb (7),
    the badge tour (8), the League verb (9).

Groups A-C are a week's interest payment and they are what make group D's
result readable. Group D is the slice's real work. Group E is slice 11 and
beyond unless S9-12 says otherwise — and note that three of its five items
are explicitly gated on measurements still running.

**Explicitly OUT of slice 10:** writing the Elite Four's requirements into Go
(slice 9 item 4's rule stands — the game states them out loud and
`Knowledge.Requirements` now harvests them); HM field moves as a general verb
(sized off item 7's survey, not before); the Safari Zone beyond the GOLD_TEETH
pickup; trading; and any change to the intent slot before S9-12 reads it.

---

## What changed on the slice 9 branch, so nothing gets rebuilt

Branch `agent-plan/0f08e43a`, unmerged, through `eba69eb`:

- `6d5665d` S9-1: `Objective.Flee` as a reply ARGUMENT (not a second menu entry
  per place — the menu is already 20+ and latency scales with it), applied by
  `WithArgs` only to kinds that travel.
- `68a9c67` S9-2: `KindUseItem` — field medicine without walking to a Center.
- `d8d70ee` S9-3: **`docs/ROAD-TO-ELITE-FOUR.md`**, the survey items 6-9 are
  read from. 235 lines, every claim cited to `pokered/<file>:<line>` or
  measured by BFS over `world.Build`.
- `83cd664` S9-4: `Objective.Intent` (200-byte cap, typed rejection),
  `Observation.Intent`/`IntentAge`, carried by Run, declared in the schema.
- `3e80d19` S9-5: `agent/memory.go` — versioned knowledge persisted beside each
  checkpoint state, atomic write, `Budget.ResumeFrom`. Adjacency deliberately
  NOT persisted (rebuilt from the ROM every run).
- `6e126b1` S9-6: `Knowledge.Requirements` — the game's own requirement
  sentences, harvested verbatim through a whole-phrase shape filter, each shape
  cited to a real ROM text. `Offer` never reads them; memory version 1 → 2.
- `f173bc2` S9-8: the drought-window measurement (item 2), and
  `NoEncounterDiagnostic` replacing an unreachable diagnostic.
- `eba69eb` S9-9: the retreat line at half max HP, `ErrTrainRetreat`, exempt
  from consecutive-failure accounting because the party state changed.

And on `main` since slice 9 was raised: `17edf44` (pick the move that does the
most damage, not the biggest number), `d93ebc9` (`Result.Err` is why the run
stopped, not the last thing that went wrong), `835a010` (the wall shows the
latest plan and newest runs first).
