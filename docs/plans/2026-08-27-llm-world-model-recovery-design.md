# PokePilot LLM world model and recovery — design

Status: approved, not implemented.
Date: 2026-08-27.

The LLM remains a high-level objective planner. PokePilot's existing skills and
controllers continue to own battles, menus, dialogue, pathfinding, and button
presses. This design improves the facts and outcomes available at the existing
`Objective`/`Execute` boundary; it does not add a campaign layer above it or
move low-level control into the model.

---

## 1. Decisions and non-goals

Keep the existing control ladder as a name for code that already exists:

1. `agent.Objective`, `Offer`, and `Execute` choose and execute semantic work.
2. Skills such as `Travel`, `Battle`, `Train`, and `StatAwareMove` implement
   game tactics.
3. `WalkPath`, `Face`, and menu helpers execute deterministic controls.

The ladder is documentation, not a new abstraction.

This design deliberately does not add:

- campaign-level actions such as `earn_boulder_badge`;
- capability profiles before a second control profile exists;
- a region graph while edge-keyed route search solves the measured Route 2
  map-reentry case;
- a ROM-hash-keyed world cache while only one ROM is supported;
- tile grids, coordinate dumps, or the full world graph in the LLM prompt.

The first implementation targets the exact supported Pokemon Red ROM.

---

## 2. Keep the two-stage navigator

Global routing continues to search the map-edge graph. Tile-level pathfinding
continues to prove whether the next edge or destination is walkable from the
player's current tile. `GoTo` re-reads live state after each leg and re-plans.

The edge-keyed `FindRouteAvoiding` search can express:

```
Route 2 south -> Viridian Forest -> Route 2 north -> Pewter
```

No region graph is needed for that route. Unwalkable legs remain facts about a
leg from a particular current position, not permanent facts about an edge.

`GoTo`'s current re-plan exhaustion must gain a distinct
`ErrReplanExhausted`. The error remains terminal even if it retains the final
`ErrLegUnwalkable` as its cause. Without the new sentinel, the current `%w`
makes exhaustion look like an ordinary recoverable unwalkable leg.

---

## 3. First priority: live sprite blockers

The static collision grid cannot know where NPC sprites stand. Sprite RAM does.
Decode the sixteen 16-byte slots across both sprite-state arrays:

- slot 0 is the player and must never enter the blocker set;
- slots 1 through 15 are candidate NPC/object sprites;
- in `wSpriteStateData1` (`0xC100`), byte `+0x00`
  (`SPRITESTATEDATA1_PICTUREID`) must be non-zero and byte `+0x02`
  (`SPRITESTATEDATA1_IMAGEINDEX`) must not be `0xff`;
- picture ID zero means an unused slot; image index `0xff` means a hidden,
  removed, or off-screen object and must not block navigation;
- in `wSpriteStateData2` (`0xC200`), byte `+0x04` is MAPY and byte `+0x05`
  is MAPX;
- MAPY/MAPX contain the object-table `+4` encoding, so subtract four when
  converting them to the tile coordinates used by `wYCoord`, `wXCoord`, and
  `world.Grid`.

Do not use `SPRITESTATEDATA2_PICTUREID` at `+0x0d` as liveness. It is scratch
used while loading sprite tile patterns and is zeroed afterwards. `wNumSprites`
is still unnecessary because unused data1 slots have picture ID zero.

`rom.ParseMap` already performs the same normalization for
`MapHeader.Objects`; no parser change is required.

The `IMAGEINDEX != 0xff` filter intentionally makes the overlay screen-local.
A sprite beyond the active screen region appears only when the player gets near
it. Every path plan reconstructs its blocker set from a fresh RAM snapshot. If
a sprite becomes visible or moves between planning and stepping, movement may
still collide. That collision causes a bounded retry which re-reads the player
and sprite positions and plans again.

`TryWalking` writes an NPC's destination MAPY/MAPX at the start of its 16-frame
walk animation. The sprite can therefore visually straddle two tiles while the
overlay contains only its destination. The same bounded retry absorbs that
race; the overlay is current engine state, not a promise that collision is
impossible.

The permanent invariant is:

> Live sprite positions are ephemeral observations, never learned world
> geometry. Every plan rebuilds blockers from current RAM. A collision retries
> from new RAM; no blocker cache exists, so there is nothing to expire or
> forget.

Keep the retry. Delete the memory of guessed blocked tiles.

---

## 4. Map names before a larger world object

`Observation.MapName` already exists but `Observe` never fills it. Maintain a
plain Go map-ID-to-name table and test it against the vendored
`pokered/constants/map_constants.asm`, following the existing
`event_constants_test.go` parity-test pattern. Unknown IDs remain `""`.

This directly improves model prompts. It does not require fixtures, emulator
boot, a world cache, or exposing more map data to the model.

---

## 5. Semantic landmarks, then `World`

The current `places` table is already the semantic layer. Its comments explain
why each destination is standable and how it relates to an NPC, counter, door,
or story location. Promote a place to a landmark record only when a concrete
objective needs more contract data:

- stable name and destination;
- approach or interaction information;
- actual availability prerequisites;
- expected objective postconditions.

`rom.MapHeader.Signs` and `Objects` are parsed inventory, not debt. Wire them
only when a concrete objective consumes them.

Introduce a shared `World` owner when graph, landmarks, parsed objects/signs,
and live overlays need one cohesive boundary. Its justification is ownership
and composition, not speed: emulated gameplay frames dominate the cost of
re-parsing map data.

The LLM receives named destinations, nearby relevant interactables, current
prerequisite facts, and recent objective outcomes. It never receives raw grids
or hundreds of coordinates.

---

## 6. Structured objective results

Three of the desired objective-contract properties already exist:

- `Offer` centralizes preconditions and distinguishes possible from wise;
- the existing skill sentinels and `%w` wrapping preserve typed failures;
- `Objective` is a small tagged union with typed Go fields.

The two real gaps are structured results and objective-boundary
postconditions.

`agent.Result` already names the whole-run result, so execution returns an
`ObjectiveResult`:

```go
func Execute(...) (ObjectiveResult, error)
```

The exact fields may follow the current objective kinds, but the result must
carry at least:

- the attempted objective;
- a normalized outcome code;
- a short planner-facing summary;
- the final structured observation;
- relevant skill output for every objective kind that produces it: complete
  `TravelResult`, complete `TrainResult`, and the Gym battle result;
- postcondition failure or unavailability in the normalized outcome code.

`Execute` must not print gameplay outcomes. Structured return data is the
outward channel. Tests prove the positive result data rather than capturing
stdout to prove a negative. This removes all four current objective-level
prints: travel blackout, training blackout, Gym win, and Gym loss.

Skills retain their existing internal positive assertions. `Execute` adds only
missing semantic postconditions, such as exact destination arrival. A skill
which returns success but whose objective postcondition is false has violated
the boundary and is terminal. A postcondition which cannot be evaluated after
its settle budget is separately terminal; unavailable is not passed. Do not
also add a `PostconditionStatus`: `completed`, `postcondition_failed`, and
`postcondition_unavailable` are the single source of truth.

---

## 7. Dialogue normalization without autonomous choices

`WalkPath` returns `ErrDialogueInterrupted` as soon as a text box appears.
At that instant `FontLoaded != 0`, so `state.Controllable` is necessarily false.
The run loop cannot classify the raw error as recoverable. The execution layer
must first normalize the interrupted state.

Preserve this invariant verbatim above both movement and recovery code:

> Movement never advances dialogue. After dialogue has interrupted movement,
> the recovery layer may press A only while ordinary text is active. It never
> answers a choice.

Refactor the existing `advanceUntil` loop rather than create a second input
loop. After movement has stopped, normalization:

1. captures the current dialogue text and failure details;
2. snapshots state before every possible A press;
3. detects a live choice before pressing;
4. taps A only for ordinary text;
5. otherwise steps frames while a script or force-walk runs;
6. stops within a fixed budget on one of four positive outcomes.

The four outcomes are:

- control restored in the overworld -> normalized `blocked` result;
- a choice is detected -> normalized `choice_required` result, without input;
- budget exhausted -> terminal stabilization failure;
- battle or another unexpected mode appears -> terminal ownership failure.

`state.DecodeMenu` alone cannot identify an open menu; its cursor fields may be
stale and it has no open/closed signal. `TextBoxID` is also not a liveness bit:
the repository already measured it staying `0x01` before, during, and after
ordinary dialogue, and `DisplayTextBoxID` does not clear it. A stale
`TWO_OPTION_MENU` (`0x14`) combined with a later ordinary text box is therefore
the dangerous false positive, not stale menu RAM after `FontLoaded` clears.

Choice detection must be anchored in what is currently drawn. Measure a
positive tilemap shape: `FontLoaded != 0`, a live menu cursor tile, and one of
the known two-option string pairs at the corresponding rows.

`ScreenText` cannot express that shape and must not be the predicate's input.
It calls `DecodeTiles`, which maps unknown tile IDs to a space and then
collapses whitespace with `strings.Fields`. The menu cursor is `▶`, charmap
`$ed`, which has no `textChars` entry: `ScreenText` renders it as a space and
then discards it along with every row boundary. A choice decoder therefore
reads the raw `sym.TileMap` bytes as 20-wide rows and tests tile IDs directly.
`ScreenText` remains the right thing to put in a planner summary or a log line.

`TextBoxID` and cursor/max RAM may be logged as supporting evidence; they
cannot be the deciding predicate.

Before general recovery uses the predicate, emulator verification must include
the adversarial state `FontLoaded = 1`, stale `TextBoxID = 0x14`, and ordinary
dialogue on screen. That state must classify as ordinary text, not a choice.

Wild battles remain owned by `Travel`. If `ErrBattle` or
`ErrBattleInterrupted` escapes an objective that should have used `Travel`, it
is a terminal ownership defect, not a planner-visible tactical choice.

---

## 8. Normalized failure policy

The run loop classifies normalized objective outcomes rather than maintaining a
table of every low-level skill sentinel.

| Outcome | Run-loop handling |
|---|---|
| Completed and postcondition true | Continue normally |
| Known gameplay blockage, stabilized to controllable overworld | Give the result to the planner and re-plan |
| Choice required, with no choice made | Give the result to the planner when a matching objective can own the choice; otherwise stop cleanly |
| Failed to stabilize | Terminal |
| Raw battle interruption escaping `Travel` | Terminal ownership defect |
| `ErrMenuStuck` or `ErrCutsceneTimeout` | Terminal controller uncertainty |
| `ErrReplanExhausted` | Terminal, even with `ErrLegUnwalkable` attached |
| Claimed success but postcondition false | Terminal invariant failure |
| Postcondition unavailable after settle budget | Terminal |
| Unknown error | Terminal by default |

The state gate is necessary but not sufficient. A recoverable result must end
controllable and outside battle, and its owning layer must have normalized the
interruption. Explicit terminal overrides take precedence.

---

## 9. Planner history and the existing stuck guard

The planner receives a short history of normalized `ObjectiveResult` summaries,
not only selected objective names. This complements the current fix for a
temperature-zero, memoryless planner; layering did not cause that oscillation.

Preserve today's no-progress counter exactly. It increments whenever
`sameProgress(before, after)` is true, regardless of which objective ran, so
alternating objectives cannot evade it.

Do not add a repeated-outcome counter. Consecutive identical resulting progress
observations necessarily become no-progress rounds after the first occurrence;
a separate fingerprint counter detects no distinct loop class and can beat the
existing guard by at most one round when the first occurrence moved before
failing. Keep the fingerprint

```
objective + normalized outcome code + resulting progress observation
```

in the diagnostic log instead. `MaxRounds` and `MaxFrames` remain outer safety
budgets.

The current `Run` comment that says no failed objective is ever retried is
load-bearing documentation. Change it with the behavior and state the
normalization, state gate, terminal overrides, and unchanged no-progress guard
explicitly.

---

## 10. Verification

### Sprite decoding and navigation

- Synthetic tests prove slot 0 is excluded, data1 picture ID zero is unused,
  data1 image index `0xff` is hidden/off-screen, and the cleared data2 picture
  scratch byte is irrelevant.
- A real fixture at a stable map-entry point anchors coordinate decoding to an
  independently parsed, already-normalized `MapHeader.Objects` entry. This
  catches a wrong stride, wrong offsets, or a missed `-4`; a synthetic test
  cannot catch a shared wrong assumption in its writer and decoder.
- A live-blocker test proves the initial path avoids occupied sprite tiles.
- A race test moves a sprite between planning and stepping, produces a
  collision, then proves the retry re-reads RAM and re-plans.
- A separate forgetting test proves a tile is immediately usable after a
  sprite moves away. This guards the absence of a blocker cache.
- Reuse `TestWalkAroundGivesUpAfterMaxRetries` for the bounded-retry assertion;
  do not duplicate it.

### Map names and objective results

- Pure table tests cover map IDs `0x00`, `0x28`, and `0x02`, plus unknown ->
  `""`; no emulator fixture is needed.
- Result tests prove `TravelResult`, `TrainResult`, and the Gym battle result
  survive into `ObjectiveResult`.
- Postcondition tests separately cover completed, failed, and unavailable
  outcome codes without a second status field.

### Normalization and run-loop policy

- Tests prove the three non-terminal actions (`completed`, `blocked`, and
  `choice_required`), that every other/unknown outcome stops, and that terminal
  overrides take precedence.
- Dialogue tests prove movement never presses A, recovery presses A only after
  an interruption, a tilemap-confirmed choice receives no input, and stale
  `TextBoxID = 0x14` during ordinary text does not stop recovery.
- Tests preserve the unchanged no-progress counter, including the alternating
  objective case. Outcome fingerprints are diagnostic output only.

### Real-ROM measurements

Everything after the live-sprite decision is source- and decomp-verified design,
not yet running-game proof. Measure these before enabling general recovery:

1. Viridian's old-man text reaches controllable overworld state within a
   bounded number of guarded A presses.
2. The tilemap-based choice predicate distinguishes a live choice from
   ordinary text even when `FontLoaded = 1` and `TextBoxID` is stale `0x14`.
3. Stabilization and choice-detection budgets are sufficient.
4. The old man's force-walk deposits the player on a predictable safe tile
   which does not immediately retrigger the gate.

Then rerun the Route 1 and Pewter journeys. Sprite timing makes the exact
collision sequence nondeterministic; the milestone proves journey completion,
not that one collision occurred. On failure, dump map/position, decoded sprite
tiles, the current blocker set, the normalized objective result, and the typed
error.

---

## 11. Delivery order

1. Add `ErrReplanExhausted` and prove its terminal precedence.
2. Measure and decode live sprites; anchor coordinates against the ROM.
3. Replace guessed tile bans with fresh RAM blockers plus bounded re-read/retry.
4. Populate `Observation.MapName` from pure map data.
5. Add `ObjectiveResult` and missing objective postconditions.
6. Measure and implement guarded dialogue normalization and choice detection.
7. Feed normalized results back to the planner while preserving the existing
   no-progress counter and logging outcome fingerprints.
8. Promote places into richer landmarks only when a concrete new objective
   needs prerequisites or postconditions; introduce `World` at that point.

Each step must retain deterministic skill tests. Real-ROM claims require the
corresponding live measurement rather than source inspection alone.
