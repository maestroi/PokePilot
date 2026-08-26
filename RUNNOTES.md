# RUNNOTES — skill tests: load checkpoint fixtures instead of replaying the story

## What changed (S5-1)
- skill/goto_test.go: TestGoToViridianPokecenter now starts from the
  pallet_town fixture (fixture.Load) instead of reds_bedroom + GetStarter.
  The Travel leg to the pokecenter, all postcondition assertions, Face(3,2)
  and the nurse Talk are unchanged.
- skill/travel_test.go: TestTravelPalletTownFightsNothing starts from the
  post_starter fixture (the state GetStarter leaves, in Oak's lab); using
  pallet_town would make destination == start and the walk vacuous.
  TestTravelPalletToViridian ALSO starts from post_starter, not
  pallet_town (see gotcha below). TestTravelMaxBattlesZero and
  TestTravelNonsenseDestination untouched: they never replay the story.
- skill/interact_test.go, skill/menu_test.go: NO changes needed. Every test
  in them already uses the cheap reds_bedroom fixture and never calls
  GetStarter/Travel; they run in 0.0-0.4s.
- No non-test file touched, fixtureVersion unchanged, no new fixtures.

## Measured (POKEMON_RED_ROM = /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb, -count=1)
- BEFORE: go test ./skill/ -count=1 = 41.8s (hot cache; cold 43.3s).
- AFTER:  go test ./skill/ -count=1 = 18.0s. ~2.3x faster, not "a few
  seconds": the remaining cost is in files this task may not touch —
  TestGetStarter 7.7s (story_test.go, replays the story by design),
  TestBootIsRepeatable 3.2s + TestBootToOverworld 1.6s (boot_test.go),
  TestCutsceneEnduresOakGate 1.0s (cutscene_test.go).
- Full suite go test ./... -count=1: exit 0, all ok, ZERO skips, zero
  failures (skill/fixture 1.7s hot after its one-time build).
- No POKEMON_RED_ROM: all 19 ROM-gated tests --- SKIP cleanly, exit 0.

## Gotchas for the next task
- Wild encounters are DETERMINISTIC per loaded state: gomeboy preserves the
  Z80/wRandom LCG phase across SaveState/LoadState. The route a test walks
  must start from the same state to fight the same battles.
- pallet_town is NOT a drop-in start for TestTravelPalletToViridian: from
  the town entry (5,6) Route 1's grass throws ZERO encounters,
  deterministically (measured 3x), so Battles >= 1 can never hold there.
  post_starter (lab) reproduces the original walk: exactly 1 battle.
- A one-time all-failure blip (every test FAIL 0.00s, ~6ms) appeared once
  between otherwise green runs; a straight re-run was green. If you see it,
  re-run before debugging the code.

## Next task
- The 18s floor is now in story_test.go/boot_test.go/cutscene_test.go.
  post_starter already exists as a fixture for anything that needs the
  post-story state without replaying GetStarter.
