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
window — never the whole map. Two optional variables:

- `PROBE_TO=11,0` — path to one tile instead of reporting every edge.
- `PROBE_BLOCK=14,13;15,12` — treat tiles as occupied, which is how you ask
  what a sprite standing in the way actually costs.

The same rule applies to the planner's prompt: whoever is reasoning gets the
answer, and deterministic code keeps the geometry.

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
spending the whole budget proving it.
