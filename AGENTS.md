# Working rules for agents

For the in-game agent loop (`-planner llm`, objectives, seeds) see `docs/AGENT.md`.
This file is about working *on* this repository.

## Never read a collision grid into context

Route 2 is 20x72. Viridian Forest is 34x48. Nothing is meant to read those
tiles but a breadth-first search, and an agent that reconstructs one by hand
runs out of context before it reaches an answer. That is not hypothetical:
three attempts at S5-3 died at 142k/150k tokens establishing reachability
and produced no commits between them. The fix was twenty lines once someone
ran the search instead of simulating it.

So run the search and read the answer:

    POKEMON_RED_ROM=roms/pokemon_red.gb PROBE_MAP=0x0c PROBE_AT=15,13 \
        go test ./skill -run '^TestProbe$' -v

`PROBE_MAP` is a map id (`0x0c` or `12`); `PROBE_AT` is where you stand. It
reports whether that tile is walkable, the nearest reachable tile on each of
the four map edges, any object sprite home tiles nearby, and a small grid
window — never the whole map. Four optional variables:

- `PROBE_TO=11,0` — path to one tile instead of reporting every edge.
- `PROBE_BLOCK=14,13;15,12` — treat tiles as occupied, which is how you ask
  what a sprite standing in the way actually costs.
- `PROBE_ROUTE=0x2f` — the legs Travel would take from `PROBE_MAP` to that
  map. Needs no `PROBE_AT`; it is a question about the map graph. With a
  standing tile it also reports whether the FIRST leg is walkable from there,
  which is the leg that fails.
- `PROBE_STATE=<path to a .state>` — read where the player ACTUALLY is out of
  a save state (a fixture under `skill/fixture/testdata/fixtures/`, or one a
  run wrote). `PROBE_MAP` and `PROBE_AT` then default to the live map and
  tile, so the common case names no coordinates at all. This is the half the
  ROM cannot answer: "why did I end up here" is RAM, not geometry. Do not
  write a throwaway test that boots a fixture and prints a position — this is
  that test.

Every run also prints that map's warps and connections, so which exit leads
where never has to be assembled by hand:

    PROBE_MAP=0x33 -> warp (15,47) -> map 0x0032 warp 1  (and the other five)
    PROBE_MAP=0x0d PROBE_ROUTE=0x2f -> leg 1: map 0x000d -warp(3,11)-> 0x002f
    PROBE_STATE=.../post_starter.v3.state PROBE_ROUTE=0x02
        -> live: map 0x0028 (5,6) facing=down controllable=true
        -> leg 1: map 0x0028 -warp(4,11)-> map 0x0000
             from (5,6): 6 step(s) to that warp, err=<nil>

The same rule applies to the planner's prompt: whoever is reasoning gets the
answer, and deterministic code keeps the geometry.

That rule binds your *thinking*, not only your context. Uncertainty about game
state is a measurement, not a deliberation: if you have reasoned two paragraphs
about where a sprite stands or whether a tile is walkable, stop and run the
probe. A second "let me reconsider" about the same fact means you owe a
command, not another paragraph. Measured on one 91-minute session here: 9,450
reasoning tokens spent guessing the Viridian Forest sprite layout, 482 "let me
reconsider", and zero probe runs.

Batch independent reads, greps and probes into one call. Every tool result
restarts the reasoning, so ten separate lookups cost ten times the thinking of
one call that answers all ten.

## Read the decomp; it is vendored here

The full pokered decomposition is at `pokered/` in every worktree — no setup,
no network. It writes its own paths as `scripts/Foo.asm`; here they are
`pokered/scripts/Foo.asm`, and several attempts have burned budget
rediscovering that. `docs/POKERED.md` maps question -> file.

Read how a value is **written**, not only how it is read. Sprite map
coordinates are stored with +4 added (`macros/scripts/maps.asm`), which is
invisible if you only read the code that consumes them.

Event flag indices come from `state.Event` or
`red/state/testdata/event_constants.asm`, **never** from counting `const`
lines: that file is full of `const_skip` and `const_next`, and hand-counting
it is how S3-1 shipped a bit index wrong by two.

## Facts that have already cost this project time

- A predicate asserts something POSITIVE about the state you want, never
  merely the absence of what you do not want.
- Use `Travel`, not `GoTo`, for anything crossing tall grass. `GoTo` aborts
  on a wild battle BY DESIGN and that stays true.
- Coordinates come from `skill.Place`, never literals.
- If input stops working, read `wJoyIgnore` (`0xCD6B`) and find the script
  that set it before blaming the emulator.
- A fixture cache that can hold a state from older code poisons itself:
  validate on write AND read, and bump `fixtureVersion` when you add one.
- Never commit a ROM or any `.gb` / `.sav` / `.state` file.
- Sprite positions are ephemeral observations, never learned geometry. Every
  plan rebuilds blockers from current RAM; no blocker cache exists, so there
  is nothing to expire or forget. A ban that outlives one plan is a bug —
  it has already shipped once.

## A journey test that loses a battle is probably not your bug

The RNG is seeded from `rDIV`, the hardware divider register
(`pokered/engine/math/random.asm`): every random number mixes in the cycle
count. So changing how many frames *anything* waits — a settle budget, a
button hold, a retry bound — reseeds every wild encounter, crit and accuracy
roll that happens after it. Fixtures are replayed rather than committed, so
the suite is one long cycle chain: an edit in `move.go` can make
`TestGymBoulderBadge` lose to Brock without touching a line it executes.

Two consequences.

**Re-running is not reproducing.** The second run is a different game, so
"let me try it again and see" answers nothing. Every `fixture.Load` test
writes its final save state to `skill/failure/<TestName>.state` when it
fails, and logs the path. Read that instead:

    PROBE_STATE=failure/TestGymBoulderBadge.state \
        go test ./skill -run '^TestProbe$' -v

(`PROBE_STATE` resolves relative to `skill/`, the test's working directory,
not the repo root. The dump's log line prints an absolute path; paste that.)

**A game outcome is not a defect.** A blackout, a lead that ended under the
level it needed, a lost battle — that is the game answering, and the dumped
state says which happened. If it is not on your task's surface, report it in
one line and move on. Adopting an unrelated journey failure is how a task
spends its whole budget inside someone else's slice.

Assertions that are a function of ROM bytes do not belong in a journey test
at all. `world` and `red/rom` tests run in milliseconds and cannot flake;
put geometry there and leave the emulator for RAM and timing.

## Throwaway measurements

Name scratch measurement files `skill/zz_*_test.go`. They are expected to be
deleted before the work is committed, and a stray one failing later is
confusing. `skill/probe_test.go` is the exception: it is permanent.

Write verbose test output to a file and grep it rather than into context.

## If you are run by agent-runner

**Do not commit.** Leave your edits uncommitted in the worktree. The runner
records the diff, runs the declared verification, and commits it itself; a
clean tree trips the no-changes gate and fails the run even when the
verification passes. This has cost this project several runs.

Reporting "this task is built on a wrong assumption" is a good outcome, and
has been the right answer more than once. Stop and say so rather than
spending the whole budget proving it. A red test that is not on your task's
surface is the same shape: name it and hand it back, do not adopt it.
