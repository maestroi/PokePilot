# RUNNOTES — fixture: cache the post-story checkpoints (done)

## What changed
Commit 6c5d63c "fixture: cache the post-story checkpoints" (on top of 00ef569).

- `skill/fixture/fixture.go`: fixtureVersion 2 -> 3 (invalidates stale v2
  caches: collision, facing decode, and the story-before-checkpoint all
  changed since v2). Registered four builders:
  - post_starter: GetStarter only (ends in Oak's lab, map 0x28).
  - pallet_town: GetStarter + GoTo Place("pallet town") (no grass, safe).
  - viridian_city / viridian_pokecenter: GetStarter + Travel
    Place("viridian city") / Place("viridian pokemon center"),
    StatAwareMove, maxBattles 20 (measured: 1 battle, no blackout).
  All destinations come from skill.Place — no coordinate literals.
  ROM bytes come from e.ROM() (emu.Open read the file once); GetStarter is
  idempotent so it runs first in every builder. Validation unchanged: a
  non-Controllable generated/cached state is still rejected and regenerated.
- `skill/fixture/fixture_test.go`: TestCheckpointFixturesAtPlace asserts
  each of the three place-built fixtures lands exactly on its Place entry
  and is Controllable; TestPostStarterFixture asserts
  EventBattledRivalInOaksLab + Controllable (no Place entry for the lab).

## Verified (POKEMON_RED_ROM = /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb)
- `go test ./... -count=1` twice, both fully green, ZERO skips.
  - Cold (fixtures built by walking the story): 45.4s total;
    skill/fixture package 45.1s.
  - Hot (fixtures cached): 42.8s total; skill/fixture package 1.7s —
    ~26x faster, caching works. Full-suite delta is small because the
    skill package (42.5s) now dominates and its tests still replay the
    story from reds_bedroom; pointing them at the new checkpoints is the
    follow-up, not this task (file scope was the two fixture files).
- git status clean; no .state files tracked (testdata/fixtures gitignored).
- goto.go, travel.go, battle.go, policy.go untouched.

## Gotchas for the next task
- Run with POKEMON_RED_ROM set or every ROM-gated test skips; the ROM lives
  at /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb
  (sha1 ea9bcae617fdf159b045185467ae58b2e4a48b9a, matches DESIGN.md).
- Use -count=1 when comparing run durations: Go's test cache otherwise
  makes the second run instant for a different reason.
- post_starter ends in Oak's lab (0x28), NOT Pallet Town; pallet_town is
  the fixture to use for route tests. Travel is mandatory for anything
  crossing Route 1 (GoTo aborts on the first wild battle by design).

## Next task
- The skill package tests can now load post_starter/pallet_town/
  viridian_city/viridian_pokecenter via fixture.Load instead of replaying
  GetStarter/Travel in each test (e.g. TestGoToViridianPokecenter from the
  old handoff). Expect the skill package to drop from ~42s toward the
  fixture package's ~2s.
