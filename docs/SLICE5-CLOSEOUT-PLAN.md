# Slice 5 close-out — prove the Boulder Badge

Status: **draft plan in agent-runner** — `db45f28c-0998-4c13-8c00-25349535c694`.
Baseline commit: `e1df491`.

**The runner plan is the source of truth.** Task instructions, measured ground
truth and verification commands live there; this file is the map. If the two
disagree, the runner plan wins and this file is stale.

S5c-1 below is deliberately NOT a runner task — it is a by-hand measurement,
and the runner plan starts at S5c-2.

Slice 5's goal (`docs/SLICE5-PLAN.md`) is the Boulder Badge. `skill.Gym` landed
(`eba1a16`) and `TestGymBoulderBadge` reaches the badge — but only because the
**test** hand-drives three things production code cannot do. This slice moves
what belongs in production into production, and states plainly what does not
close here.

## What the milestone is actually standing on

`skill/gym_test.go` reaches Pewter with two helpers and a retry wrapper:

- **`crossGate`** — the two forest gate maps (`0x32` south, `0x2F` north) each
  expose warps at `(4,0)` and `(5,0)` to the same destination
  (`pokered/data/maps/objects/ViridianForest{South,North}Gate.asm`). `(4,0)` is
  non-walkable in this ROM (measured), and the pathfinder picks it, so the
  automatic leg reaches the gate and stops. The test walks to `(5,1)` and holds
  up onto the real `(5,0)`. **This is a production routing bug wearing a test
  helper.**
- **`travelFightsThrough`** — retries `Travel` up to ten times, calling
  `dismissDialogue` each time it returns `ErrDialogueInterrupted`. Ten is not
  decoration: signs, gate NPCs and the rival cutscene interrupt this route
  repeatedly.
- **`dismissDialogue`** — taps A until the box clears or a battle starts. It
  would answer a yes/no blindly. Acceptable in a test that knows the route;
  not shippable.

So `TestGymBoulderBadge` passing does **not** mean `Travel` can reach Pewter.
`TestTravelToPewter` (preserved in `RUNNOTES.md`) is a single bare `Travel`
call, which is why it is the honest milestone — and why it cannot pass until
the dialogue work lands in Slice 6.

## The scope decision, made explicitly

Two things stop a bare `Travel`: the gate warp choice, and dialogue
interruption. They are not the same size.

- The **gate warp choice** is small, is the same family of bug as the
  band-split fix already committed in `871f9d4` (the graph knows connections,
  not reachability; the tile pathfinder must arbitrate), and deleting
  `crossGate` is its own proof. **It closes here.**
- **Dialogue recovery** cannot be done safely without a predicate that tells a
  question from a statement, which does not exist yet — see
  `docs/SLICE6-PLAN.md` S6-5. Pressing A blindly is what `dismissDialogue`
  does, and shipping that is worse than not shipping it. **It moves to Slice
  6**, and `TestTravelToPewter` moves with it.

This corrects earlier sequencing advice that treated dialogue recovery as
deferrable robustness. It is not: it is what stands between a hand-driven
milestone and an autonomous one.

## Hard preconditions

- Branch from `e1df491` or later. Branching from `4506eac` reintroduces the
  band-split hole and the worker will chase a fix that already exists.
- `POKEMON_RED_ROM` must be set for every task that names a fixture. Without
  it those tests skip, and a skip is not a pass.
- Under agent-runner: do not commit (`AGENTS.md`).

---

## S5c-1: Measure what actually stops a bare `Travel`

**Run by hand. This is a measurement, not an agent-runner task.**

A task whose text says "find out why" spends its whole budget finding out —
this project has paid that bill three times (`docs/SLICE5-PLAN.md`, process
note).

Restore `TestTravelToPewter` verbatim from `RUNNOTES.md` (plus the `red/state`
import) into `skill/travel_test.go`, then:

```bash
POKEMON_RED_ROM=roms/pokemon_red.gb go test ./skill -run TestTravelToPewter -count=1 -v \
  > /tmp/pokepilot-s5c-1.log 2>&1; tail -40 /tmp/pokepilot-s5c-1.log
```

It is **expected to fail**. The deliverable is the list of what stops it and
where, in order — gate, text box, sprite, or something not predicted here.
Record each stop's map, tile and typed error at the top of this file.

Then mark the test `t.Skip` with a pointer to `docs/SLICE6-PLAN.md` rather
than deleting it again. A deleted test is how this milestone got lost once
already.

**Tools:** the preserved test is already written; `skill/probe_test.go`
(`PROBE_MAP`/`PROBE_AT`/`PROBE_BLOCK`) answers any "is that tile reachable"
question without reading a grid into context.

---

## S5c-2: `ErrReplanExhausted`

Task 1 of `docs/plans/2026-08-27-llm-world-model-recovery.md`, unchanged.

`skill/goto.go:88` wraps only `ErrLegUnwalkable`, so exhausting the re-plan
budget is indistinguishable from one recoverable leg. Small, pure, no ROM.

**Files:** `skill/goto.go`, `skill/goto_replan_test.go`.
**Verify:** `go test ./skill -run 'TestReplanExhausted|TestWalkAroundGivesUp' -count=1`

---

## S5c-3: Traverse a warp the pathfinder can actually reach

**The task that deletes `crossGate`.**

Both forest gates expose two warp tiles to the same destination map. The graph
offers whichever it built first; `(4,0)` is non-walkable, so the leg dies on a
gate that has a perfectly good exit one tile right.

Make `Traverse` (`skill/warp.go`) consider **every** warp tile on the current
map leading to the target map, and use the first the tile pathfinder can
actually reach from where the player stands. Unreachable warp tiles are a fact
about the current position, exactly like `GoTo`'s per-tile leg bans — do not
cache them as properties of the map.

**Measure first** (by hand): confirm with the probe that `(4,0)` is
non-walkable and `(5,0)` is reachable on both `0x32` and `0x2F`, and paste the
output into the task text.

```bash
POKEMON_RED_ROM=... PROBE_MAP=0x32 PROBE_AT=5,1 go test ./skill -run '^TestProbe$' -v
```

**Files:** `skill/warp.go`, `skill/warp_test.go`, `skill/gym_test.go` (delete
`crossGate` and its two call sites).
**Tools:** `rom.ParseMap` already returns the full warp table; `world.FindPath`
already answers reachability from a tile.
**Done when:** `TestGymBoulderBadge` passes with `crossGate` gone and legs 1-5
collapsed into plain `Travel` calls.

---

## S5c-4: Decode live sprite positions

Task 2 of the world-model plan, with the corrected predicate.

**The predicate is measured, not guessed.** Use `wSpriteStateData1`
(`sym.SpritePlayerStateData1`, `0xC100`): byte `+0x00` non-zero and byte
`+0x02` not `0xff`. Do **not** use `SPRITESTATEDATA2_PICTUREID` at `+0x0d`;
`pokered/engine/overworld/map_sprites.asm:223-231` zeroes it for every slot
once tile patterns are loaded. Coordinates come from `wSpriteStateData2`
`+0x04`/`+0x05` minus 4 (`pokered/macros/scripts/maps.asm` stores objects with
`+4`; `red/rom/map.go` already removes it).

**Files:** `red/state/sprite.go`, `red/state/sprite_test.go`,
`skill/sprite_fixture_test.go`.
**Verify:** `go test ./red/state -run TestDecodeSprites -count=1`, and with the
ROM `go test ./skill -run TestDecodedNurseTileMatchesROMObject -count=1`.

---

## S5c-5a: `walkAround` plans from fresh RAM

Task 3 steps 1-2 of the world-model plan. One function plus its tests.

Replace the `hit`/`blocked` guess maps with a
`readBlocked func() map[[2]int]bool` called once per attempt. Keep
`maxWalkRetries` and `npcWaitFrames`, and keep
`TestWalkAroundGivesUpAfterMaxRetries` and
`TestWalkAroundDoesNotRetryOtherFailures`.

**Files:** `skill/blockers.go`, `skill/move.go`, `skill/walkaround_test.go`.
**Tools:** the existing tests already drive `walkAround` through injected
`plan`/`walk`/`wait` seams, so the new race and forgetting tests need no
emulator.
**Verify:** `go test ./skill -run 'TestWalkAround|TestSpriteBlockers' -count=1`

---

## S5c-5b: Wire every path planner onto it

Task 3 step 3. Separate task, separate blast radius.

`skill/goto.go` and `skill/warp.go` pass `spriteBlockers(m)`. `walkLab` in
`skill/story.go` moves onto the same helper, merging its static ball-tile
exclusions into every fresh snapshot rather than caching them.

**Files:** `skill/goto.go`, `skill/warp.go`, `skill/story.go`.
**Verify:** `go test ./skill ./world -count=1`

---

## S5c-6: The milestone, with evidence on failure

```bash
POKEMON_RED_ROM=... go test ./skill -run TestGymBoulderBadge -count=3 -v \
  > /tmp/pokepilot-s5c-6.log 2>&1
```

Do not assert that a collision occurred — sprite timing is nondeterministic.
The milestone proves the badge bit and a controllable player. On failure dump
map name/id, player tile, decoded sprite slots, the current blocker set, and
the typed error chain.

**Done when:** `TestGymBoulderBadge` passes three runs in a row with the ROM
set and **without `crossGate`**, and `go test ./...` is green without the ROM.

---

## What does not close here, and why

- **`TestTravelToPewter`** — a bare `Travel` to Pewter. Blocked on dialogue
  interruption, which needs S6-5's choice predicate first. Left skipped with a
  pointer, not deleted.
- **`dismissDialogue`** — stays in the test until production recovery exists.
  It answers questions blindly; promoting it as-is would ship the exact defect
  the world-model design spends its longest section arguing against.
- **World-model plan Tasks 5, 7, 8** (structured results, normalization,
  planner history). Slice 6 owns them.
