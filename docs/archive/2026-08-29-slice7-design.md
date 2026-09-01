# Slice 7 design — make the measurement honest, then give the planner a world to read

Date: 2026-08-29
Status: draft for review

## Scope note: this is one of three sub-projects

The slice 7 discussion raised five things. They decompose into three
independent projects, and this spec covers only the first:

- **A — capability + honest measurement (THIS SPEC):** stating the goal,
  validating the model's replies and identity, the planner's rejection
  recovery, NPC interaction, item pickup.
- **B — test economics:** an artifact store holding checkpoints and failure
  dumps, and segment-based testing so the Elite 4 does not require replaying
  the whole game. Independent of A. Its seam already exists as
  `POKEPILOT_FIXTURE_DIR` in the slice 6 close-out plan.
- **C — data pipeline:** the farm at volume plus an investigation agent that
  clusters failures into issues. **Depends on B** and on this spec's item 1,
  because clustering noise industrialises noise.

B and C get their own specs. Do not start them from this one.

## Goal

Make S6-12's measurement mean something — tell the planner what it is trying
to do, and stop killing runs over reply shape — then let the planner see the
two things in the world it currently cannot: people who know things, and items
lying on the floor.

## The evidence this rests on

MEASURED 2026-08-29 from S6-12 attempt 1's own sweep logs
(`badgerun-out/baseline-console.log`, `badgerun-out/ablationA-console.log`):

| sweep | runs | ended in error | "argument does not apply" | badges |
|---|---|---|---|---|
| baseline (qwen3.5-4b) | 9 | **9** | **27** | 0 |
| ablation A (qwen3.8-27b) | 3 (sweep incomplete) | 0 | 0 | 0 |

**Every baseline run terminated on argument validation.** The model returns a
valid choice carrying a superfluous argument and the run dies:

```
llm: 4 offered, reply "{\"choice\": 3, \"level\": 12}"
  -> rejected: agent: level argument 12 does not apply to go to route 1
  -> badge=false stop=error PALLET_TOWN (5,6)
```

So a scoreboard reading "4b: 0/9" would measure a schema bug, not reasoning.
S6-12's own text requires ruling this out before blaming the model or the
information — *"Before concluding either, rule out the third possibility: the
LOOP."* This is the loop.

The 27b ablation produced zero validation errors and instead wandered
(Viridian City → Oak's Lab → Viridian City → Route 2). That is a real planning
weakness and worth measuring — but only once the baseline is not dominated by
a parse error.

## 0. The planner is never told what it is trying to do

**This is the most serious finding and it comes first.**

VERIFIED 2026-08-29. `agent/llm.go:171` is the entire system prompt:

> You are choosing the next objective for a Pokemon Red player. Prefer an
> objective that makes NEW progress: repeating what you just did wastes the
> run. Reply with ONLY a JSON object [...]

There is no goal in it. `grep -i goal` over `agent/` returns **nothing** — the
concept does not exist in the codebase. The dumped prompts confirm it
end-to-end: the only occurrences of "Badge" in a real prompt are the
observation field `"Badges": []`.

So S6-12 measures "does the planner earn the Boulder Badge" while never
telling the planner that a badge exists, or that earning one is the point.

**Goal is not solution, and the difference is the whole experiment.**
S6-12 is right to withhold the SOLUTION — it must not say "take Squirtle", or
mention Caterpie, or hint that Butterfree beats Onix. Working that out is the
capability being measured. But the GOAL is the task statement, not the answer.
Withholding it does not make the test harder; it makes it a different test —
of what an unprompted model does with a menu — and that is not the question
this project exists to ask.

This reframes the one behaviour the 27b ablation actually exhibited. Its
wander — Viridian City → Oak's Lab → Viridian City → Route 2 — is not weak
planning. It is a model with no objective correctly obeying "prefer something
new". It also undercuts ablation B: injecting "Rock-type Pokemon resist Fire"
tests a fact the model has no stated reason to want.

**Design.**

- `Run` takes a goal — one sentence of plain English, stating the task and
  nothing about how to reach it. For this milestone: `"Earn the Boulder Badge."`
  Not "train a Butterfree", not "beat Brock with a Grass or Water type".
- The goal is rendered into the system prompt above the observation.
- It is a RUN PARAMETER, not a constant, and `badgerun` takes it as a flag.
  This is what later makes segment-based testing expressible: sub-project B's
  checkpoints plus a goal give "start from this state, achieve this" — which
  is the only affordable way to test toward the Elite 4.
- Anything beyond the task statement goes through S6-12's existing
  fact-injection flag, which already defaults OFF and is already labelled a
  diagnostic that must not ship. The goal is not fact injection; keep the two
  separate so the ablation stays meaningful.

**The measurement consequence.** No scoreboard collected before this lands
says anything about reasoning. Item 0 and item 1 must both be in before
S6-12's numbers are read as capability.

## 0b. Nothing checks that the model answering is the model we asked for

VERIFIED 2026-08-29. The entire response envelope the client parses is:

```go
type chatResponse struct {
    Choices []chatChoice `json:"choices"`
}
```

No `model`, no `finish_reason`, no `usage`; `grep -c finish_reason` over
`agent/llm.go` returns 0.

Credit where due — the transport layer is careful. Non-200, unreadable body,
malformed envelope and empty `choices` all produce typed errors; an
out-of-range choice is an error and never a guess; and `answerInt` strips
reasoning blocks and takes the LAST integer, which is a real bug avoided.
The gaps are all one layer up, in trusting the reply's *content*.

**1. Model identity is never verified — this invalidates ablation A.**
S6-12's ablation A is "swap the model, hold information fixed": solves it means
capacity, still fails means not capacity. If the server ignores the `model`
field, or has a single model loaded, or the env var is wrong, the ablation
compares a model to itself and reports "not capacity". That is a FALSE
NEGATIVE on the central experiment of the project, and nothing in the code or
the scoreboard would reveal it. The response carries `model`; read it and fail
loudly when it is not what was requested.

**2. Truncation is invisible.** `finish_reason == "length"` means the reply was
cut off. A truncated JSON that still parses becomes a silent wrong answer —
relevant given the observed `{"choice": 1, "species": "}"}`. Read
`finish_reason` and treat anything but `stop` as a rejected reply (which item 1
now makes retryable rather than fatal).

**3. The plain-text fallback is too permissive.** `resolveReply` falls back to
`answerInt` whenever the JSON parse fails, and `answerInt` takes the last
integer ANYWHERE in the reply. An HTTP-200 body containing prose — "rate
limited, retry in 5 seconds" — becomes choice 5. The fallback exists for a
good reason (servers that ignore `response_format` still answer in plain text)
and must stay, but it should require the reply to LOOK like an answer: short,
and substantially just the number. A long prose reply containing a digit is an
unhealthy response, not a choice.

**4. No preflight, and no health counters in the scoreboard.** Nothing verifies
the endpoint is up and serving the expected model before a multi-hour sweep,
and the per-run record counts badges, frames and objectives but not timeouts,
HTTP errors, unparseable replies, or fallback-path uses. Those four counters
are what answer "was the API healthy for this run", and without them a bad
sweep is indistinguishable from a bad model.

This is not hypothetical. Ablation A's own launcher carries
`POKEPILOT_LLM_TIMEOUT=5m` with the comment that the 60s default "was killing
runs spuriously" — a sweep was already being corrupted by API behaviour that
nothing surfaced.

**Design.** Preflight the endpoint once per sweep (one cheap call, assert the
served model matches the requested one, fail the sweep loudly if not); verify
`model` and `finish_reason` on every reply; tighten the fallback to reject
prose; and add the four health counters to the per-run record so every
scoreboard row carries the conditions it was collected under.

## 1. The planner may be wrong about its reply without the run ending

**Where it is today.** `agent/planner.go:99,107` returns an error when an
argument does not apply to the chosen objective. `agent/run.go` classifies any
planner error as `StopError` — "a planner error, or nothing is possible from
here" — which is terminal.

**The asymmetry to fix.** `Run` already treats a failed *objective* as
information: it has a failure budget, and it deliberately weights different
failures more heavily than repeats. But a malformed *reply* is a different
kind of event — it says nothing about the world, only that the model answered
in the wrong shape — and it is retryable by asking again with the rejection
quoted back.

**Design.** Rejection becomes a retry with feedback, bounded:

- Keep the rejection itself exactly as strict. The planner picks from a menu
  and never invents; coercing `{"choice":3,"level":12}` into "go to route 1"
  would silently discard what the model asked for. That safety property is
  S6-7's and it stays.
- On a rejected reply, re-prompt the same round with the rejection text
  appended, up to `maxReplyRetries` (propose 3). The observation does not
  change; only the rejection feedback is added.
- Exhausting the retries is `StopError` as today. A model that cannot answer
  in shape three times running is a real finding, not a transient.
- Count the retries and surface them per run, because "how often did the model
  answer in the wrong shape" is exactly the diagnostic S6-12 needs to separate
  loop from capacity.

**Why not just ignore superfluous arguments.** Because `{"choice":1,
"species":"}"}` is not a model that meant "go to pallet town" — it is a model
whose output is malformed. Ignoring the argument hides that; quoting it back
gives the model a chance to correct and gives us a number.

## 2. Where the planner learns what is on a map

The naive design — offer whatever sprites are visible — is wrong, and this was
MEASURED rather than assumed.

Loading the `viridian_city` fixture puts the player on map `0x01` at (23,26).
`state.DecodeSprites` returns **zero sprites**. That is correct: the nearest
NPC is at (30,25), seven tiles away, and the decoder's `IMAGEINDEX == $ff`
filter is strictly screen-local. A planner offered only visible sprites could
never choose "go and talk to the gym guide", because the guide is invisible
until the player is already standing next to him.

**Three sources, cleanly separated:**

| source | answers | already exists |
|---|---|---|
| ROM map header (`rom.MapHeader.Objects`) | *where* NPCs, items and trainers are, map-wide | yes |
| live sprite RAM (`state.DecodeSprites`) | liveness and blockers, screen-local | yes, used for blockers |
| the Pickup postcondition itself | whether a given item ball is still there | yes — see below |

This is the same split the project already enforces: live sprite positions are
ephemeral observations, never learned world geometry.

## 3. The ROM parser already finds items and trainers, then discards them

`red/rom/map.go` parses every map object into
`Object{X, Y, SpriteID, Movement, Range, TextID}` with the `+4` coordinate
bias already removed, and it already distinguishes the three kinds:

```go
case obj.TextID&0x40 != 0: // trainer entry: 2 extra bytes
    o.skip(2)
case obj.TextID&0x80 != 0: // item entry: 1 extra byte
    o.skip(1)
```

Those skipped bytes are the payload: for an item entry, the item ID; for a
trainer, the class and roster. Capture them instead of skipping:

- `Object.ItemID uint8` — set when `TextID&0x80`, else 0.
- `Object.TrainerClass, Object.TrainerSet uint8` — set when `TextID&0x40`.

This is pure parsing, unit-testable with no emulator, against known ground
truth from `data/maps/objects/ViridianForest.asm`:

```
object_event 25, 11, SPRITE_POKE_BALL, ..., ANTIDOTE
object_event 12, 29, SPRITE_POKE_BALL, ..., POTION
object_event  1, 31, SPRITE_POKE_BALL, ..., POKE_BALL
```

## 4. Observation and offering

`Observation` gains the current map's objects:

```go
// MapObjects are the current map's objects read from the ROM map header,
// not from sprite RAM: sprite RAM is screen-local (MEASURED: zero sprites
// visible standing seven tiles from an NPC), so it cannot tell a planner
// that a person worth talking to exists across the map.
MapObjects []MapObject

type MapObject struct {
    X, Y uint8
    Kind string // "person" | "item" | "trainer"
    Item string // item name, "item" only; "" otherwise
}
```

The offering layer offers `KindTalk{X,Y}` for each person and
`KindPickup{X,Y}` for each item. Trainers are reported but not offered — see
Out of scope.

**Collected items are NOT filtered out, and that is a decision, not an
oversight.** VERIFIED 2026-08-29: `red/state` decodes exactly eight story
events by hand-picked bit index (`EventFollowedOakIntoLab` … 
`EventBeatRoute22Rival1stBattle`) and **none of them is an item pickup**, so
there is no data source for "already collected" today. The three ways out:

1. Decode the per-item event flags. Real work, a new decoder, and speculative
   until something needs it.
2. Use sprite RAM for liveness — a collected ball is suppressed via
   `IMAGEINDEX == $ff`. But that is screen-local, so it cannot answer the
   question from across the map, which is the whole point of offering.
3. **Do not filter.** Offer every ROM item position. Picking up a ball that is
   already gone fails Pickup's postcondition (the bag did not grow), which is
   an ordinary failed objective — and this project's entire design says a
   failed objective is information, not a crash. S6-9 already feeds it back.

Take option 3. It needs no new decoder, it turns missing data into a signal
the loop already handles, and it keeps the slice small. Upgrade to option 1
only if measurement shows a planner looping on a collected ball; `History`
already gives it what it needs to avoid that.

## 5. The pickup verb

`skill.Pickup(m *emu.Emu, x, y uint8) error`. The executor is the same as
Talk's — face the tile, press A — but the postcondition is different and it is
the reason this is its own Kind rather than a Talk:

- Talk's postcondition is "dialogue happened". That does not prove an item was
  collected.
- Pickup's postcondition is POSITIVE and specific: **the bag's count for that
  item rose by one.** `state.DecodeInventory` already decodes the bag.

An item ball also opens a text box on pickup, so Pickup must page it closed —
and, per the two prompts this project has already been bitten by (S6-3's
nickname box, S6-4's "forget a move?"), it must never answer a two-option menu
by reflex. Use `state.DecodeTwoOptionMenu` to detect one and stop rather than
guess; no item pickup in Red asks a question, so encountering one is a finding
to report, not a case to handle.

## 6. Milestone

One Pallet → Brock run in which the planner:

1. picks up the free POKE_BALL in Viridian Forest at (1,31), and
2. talks to the Pewter gym guide (`PEWTERGYM_GYM_GUIDE`, map object at (7,10))
   before the Brock fight.

Both are asserted from RAM: the bag gained a POKE_BALL, and the guide's line
appears in `Observation.RecentDialogue`.

This reuses the existing journey scaffolding rather than building a new one,
and it repairs S6-12's recall-vs-derivation reading: the guide is the in-world
derivation path S6-12's own text names — *"it learns it by losing or from an
NPC who says so"* — and it is currently unreachable, so "derivation" has been
measured with one hand tied.

## Out of scope, deliberately

- **Hidden items** (`hidden_event` objects). They need ITEMFINDER and a
  different mechanism entirely.
- **Trainers as a fightable objective.** The Pewter gym's second trainer
  (`PEWTERGYM_COOLTRAINER_M` at (3,6)) is real and no current test accounts for
  it, but a KindFight verb is its own slice.
- **Repeat-talk suppression.** `History` already shows the planner what it has
  tried. Inventing suppression before measuring a loop is the heuristic trap
  that S6-0f banned for HP.
- **The Pokemon Center PC.** It exists (`hidden_event 13, 3,
  OpenPokemonCenterPC`, faced UP, in every Center — correcting a false claim
  committed in S6-4's RUNNOTES), but nothing here needs to deposit a mon.

## Risks

- **A planner looping on a collected item.** Since collected balls are not
  filtered (see section 4), the planner may be offered a ball that is gone.
  One failed Pickup is fine and informative; the same Pickup chosen five times
  running is a loop. MEASURE this in the milestone run before adding an event
  decoder — the fix, if needed, is option 1 in section 4, and it should be
  bought with evidence rather than in advance.
- **Object count.** Some maps carry many objects; offering all of them could
  flood the prompt. Measure the largest count on the Pallet→Pewter route
  before deciding whether a cap is needed. Do not pre-emptively cap.
- **Sequencing.** Items 0 and 1 must both land and be re-measured before
  anything else is judged. Until item 1 lands, no scoreboard distinguishes a
  planning failure from a parse error; until item 0 lands, no scoreboard
  distinguishes weak planning from a model that was never told what to do.
- **Goal wording is load-bearing and easy to get wrong.** One sentence too
  helpful ("earn the Boulder Badge at Pewter Gym by training a Grass or Water
  type") silently converts the benchmark into a walkthrough, which is exactly
  what S6-12's fact-injection flag exists to keep out of the default path. The
  implementation plan must fix the exact wording and put it under review, not
  leave it to the implementer.
