# Slice 9 candidates — raised 2026-08-30

Raised on `main` at `0c6c9b4`, which CONTAINS all of slice 8 (merged
2026-08-30, commit `872f113`) plus four follow-up commits. Unlike the slice 8
doc, nothing here is blocked on an unmerged branch.

Labelling, as always: MEASURED means someone ran it, DERIVED means it was read
out of `~/.cache/pokered`, UNMEASURED means it needs a task before it can be
scoped.

---

## The proposed goal: a run that keeps its own plan

The goals so far have been one badge away: "Earn the Boulder Badge", "Reach
Cerulean City". Each fit inside a single round-budget and inside the planner's
own short memory. The next question is different in kind — **can a run hold an
objective longer than it can remember?** — and the Elite Four is the honest
end of that question.

It is NOT a proposal to write the Elite Four's requirements down in Go. See
item 4: the game states them out loud, and writing them down ourselves is the
same seeding the project has refused everywhere else.

---

## 0. Slice 8 built three verbs the planner cannot reach

**MEASURED 2026-08-30**, by grep, on `main` at `0c6c9b4`:

    $ grep -rn "skill.Flee\|skill.TravelFlee\|skill.UseFieldItem" agent/ cmd/
    (no matches)

`skill.Flee` (S8-4), `skill.TravelFlee` and `skill.UseFieldItem` (S8-5) exist,
are tested, and are unreachable from a planner. There is no `KindFlee`, no
`KindUseItem`, and `agent.Execute`'s switch has never heard of either. The
planner's whole vocabulary is still the ten kinds in `agent/objective.go:17`.

This is the same shape as slice 7's finding — an affordance built and then not
wired to the thing that was supposed to use it — and it is the top item for
the same reason: everything else in this slice is worth less while the verbs
the last slice paid for stay dark.

`TravelFlee` is the sharper loss. Mt. Moon is a cave full of Zubat, and every
journey through it currently fights every encounter, which is what S8-4 built
the flee path to avoid.

---

## 1. The goal is a sentence, and nothing reads it

DERIVED — `agent/llm.go:427`:

    func (p *LLMPlanner) systemPrompt() string {
        s := llmSystemPrompt + p.ExtraSystem
        if p.Goal != "" {
            s = "Your goal: " + p.Goal + "\n\n" + s
        }
        return s
    }

That is the entire implementation of `Goal`. Nothing decomposes it, nothing
checks progress against it, and no stop condition has heard of it. A run given
`-goal "Beat the Elite Four."` behaves exactly like a run given
`-goal "Earn the Boulder Badge."` — the sentence changes, the machine does
not.

---

## 2. The run re-derives its intent every round

`agent.Run` carries three things between rounds: the `Observation`, a
`Knowledge`, and `History`. None of them is a plan.

- `History` is the last few `RoundRecord`s (`appendHistory`) — objectives and
  outcomes, no intent.
- `Knowledge` (`agent/offer.go:23`) holds visited maps, spoken place names,
  completed objectives and talked-to tiles. It is what the player has SEEN,
  never what the player is TRYING TO DO.
- The `Observation` is the present tense by construction.

So every round the planner answers "what now?" from a menu and a handful of
outcomes, with no way to write down "I am partway through something I decided
forty rounds ago". Under temperature 0 that is also the loop the project has
already measured twice: an objective that returns the player somewhere they
have been is chosen again for the same reasons it was chosen the first time.

**There is no slot anywhere for a plan.** That is the gap, and it is
goal-agnostic: it is the same gap whether the goal is the Cascade Badge or the
Elite Four.

---

## 3. A run's knowledge does not survive the run

`NewKnowledge` is called once per `Run` (`agent/run.go:320`) and the result is
dropped when `Run` returns. `Budget.CheckpointDir` writes an emulator
save-state ring before every objective — the GAME survives, the RUN's
understanding does not. Restart from a checkpoint and the agent is amnesiac in
a world it has already explored.

For a one-badge goal that costs little. For a goal measured in hundreds of
rounds it means a crash is a total loss, and it means the wall cannot resume
anything.

---

## 4. The game states its own requirements out loud

This is the item that makes "figure it out" a real design rather than a wish.

DERIVED — `scripts/Route23.asm` has SEVEN badge checks (lines 155-191), one
per badge, each with its own `EVENT_PASSED_*BADGE_CHECK` flag. The guard's
line, `text/Route23.asm:1`:

    You can pass here only if you have the <BADGE>!
    You don't have the <BADGE> yet!
    You have to have it to get to #MON LEAGUE!

The game names the requirement, names the thing that satisfies it, and says
what it is for — to a player who simply walks up. It is not the only one:
`scripts/VermilionCity.asm:56` gates the S.S. Anne on `S_S_TICKET`, and
`SSAnneCaptainsRoom.asm:31` is where `EVENT_GOT_HM01` (Cut) is set.

And the harvesting channel already exists. `Knowledge.SawDialogue`
(`agent/offer.go:52`) already reads every line the game says and pulls place
names out of it, using `skill.PlaceNames()` as a vocabulary rather than a
seed — "a name enters Knowledge only when the game actually said it".

It harvests place names and nothing else. Requirements go past it unread.

**The design rule this protects.** Writing "the Elite Four needs 8 badges and
a level-70 team" into `Offer` would be the same seeding the project refuses in
`Knowledge`'s own doc comment ("never seeded from the ROM's map table or from
a list written out in advance: seeding it is the difference between an agent
that explores and an agent that reads our notes"). It would also void the
scoreboard the way slice 6's was voided: a run that succeeds because we handed
it the checklist measures the checklist.

---

## 5. What actually gates the road past Cerulean — UNMEASURED

Reach today is Cerulean City. What stands between there and Indigo Plateau has
never been surveyed against this codebase, and **must not be written from
memory** — the task-authoring rule exists because both Phase 1 failures were
wrong assumptions in a spec, not bad code.

DERIVED, the destinations exist (`constants/map_constants.asm`):

    INDIGO_PLATEAU        $09     VICTORY_ROAD_1F   $6C
    INDIGO_PLATEAU_LOBBY  $AE     VICTORY_ROAD_2F   $C2
    LANCES_ROOM           $71     VICTORY_ROAD_3F   $C6
    CHAMPIONS_ROOM        $78

DERIVED, two gates are already visible:

    scripts/VermilionCity.asm:56      S_S_TICKET gates the S.S. Anne
    scripts/SSAnneCaptainsRoom.asm:31 EVENT_GOT_HM01 (Cut) is given there

Everything else — which HM gates which map, which gyms need what, whether the
map graph can even route past Cerulean — is a survey, and the survey is a
task with a document as its deliverable, not a guess in this file.

Note the shape of the problem: `Offer` refuses to offer an objective with no
verb behind it (that is why trainers are reported in `MapObjects` and never
offered). A plan layer built over a menu that cannot reach the goal produces a
guaranteed-failure objective every round. **The planning layer and the verb
backlog are two different slices, and the verbs are the ones that gate the
goal.**

---

## 6. Cost, which is now a design constraint

MEASURED: the Pallet -> Brock journey is ~15 rounds and 87 seconds of
emulation (`TestGymBoulderBadge`, twice on 2026-08-30). MEASURED 2026-08-29:
an LLM call takes ~6-7s with 5-7 objectives offered and ~21-23s with 13-15.

The offered menu grew this week — one catch per wild species instead of one
fixed catch, a travelling heal, gyms beyond Pewter. A longer menu is a slower
round, and a goal measured in hundreds of rounds multiplies both. Any plan
layer that adds prompt text is spending the same budget.

---

## Suggested sequencing

1. **Wire slice 8's verbs to the planner (0).** Flee, TravelFlee and
   UseFieldItem exist and cannot be chosen. Smallest item, and it is paid for.
2. **Survey the road past Cerulean (5).** A document, not code. It is what
   says whether the Elite Four is three slices away or fifteen, and it sizes
   everything after it.
3. **Give the run a plan it keeps (2)**, goal-agnostic: somewhere to write
   what it is trying to do next and what it has established, carried across
   rounds and rendered into the prompt.
4. **Persist it (3)** beside the checkpoints, so a crash or a resume does not
   restart an amnesiac.
5. **Harvest requirements from dialogue (4)**, extending `SawDialogue` past
   place names — the Route 23 guard is the proof case, but it must be
   demonstrated somewhere reachable today.
6. **Measure a long goal-driven run.** How far does it get, and what stopped
   it. The answer is the input to slice 10, and "it stopped at a missing verb"
   is a successful measurement.

Explicitly OUT: writing the Elite Four's requirements into Go (item 4), HM
field moves as verbs (they are their own slice, sized off item 2 of the
sequencing), the Cascade Badge fight, trading, and the Safari Zone.

---

## What changed on main this week, so nothing gets rebuilt

Slice 8 merged at `872f113`: StepFrames runs per-frame hooks, the forget-a-move
prompt is answered on purpose, `skill.Flee`/`TravelFlee`, `skill.UseFieldItem`,
places through Cerulean, the collision-grid fix, and the wall's failure triage.

Then, uncommitted-then-committed on main the same day:

- `d815ea2` HasGrass requires a non-zero wild rate; blackout is exempt from the
  failure budget; Pickup approaches with Travel.
- `20d8270` `Observation.RespawnPlace` from `wLastBlackoutMap`, and the
  blackout round records what it cost (MEASURED: 3175 -> 1587).
- `23d7c0e` `KindHeal` takes an optional Place and travels to a known Center.
- `5f379dd` one catch per species the map's wild table holds
  (`skill.WildGrass`), and all 151 species named from the ROM's `MonsterNames`.
- `0c6c9b4` the training target is the lead's level + 2, and `skill.Gym` reads
  a gym table (Pewter and Cerulean) instead of hardcoding Brock.
