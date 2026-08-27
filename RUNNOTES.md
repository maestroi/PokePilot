# RUNNOTES — S5b-6a: Pewter arrival milestone test in skill/travel_test.go

## What landed
- skill/travel_test.go: `TestTravelToPewter` (compiles; NOT executed in this
  step). Loads fixture `post_errand` (old-man gate open, player controllable
  at (19,8) on Viridian 0x01), Travel to `skill.Place("pewter city")`
  (0x02, 14, 8) with StatAwareMove, budget 20. On error: Fatalf naming the
  stop map/X/Y and res.Battles (mirrors TestTravelPalletToViridian). On
  success: ONE state.Snapshot + DecodePlayer asserts the player is EXACTLY
  at the Place, and state.Controllable is true; both failures name
  res.Battles and res.BlackedOut. Added `red/state` import; no other file
  touched, no fixture added or bumped (post_errand exists at v4 and v5).
- Verified: `go build ./...` and `go vet ./...` green; `go test -run
  ZZZ_no_match ./skill/` links the test binary (zero tests executed).

## For the next task (running/finishing S5b-6: survive ambushes, reach Pewter)
- First action: actually RUN TestTravelToPewter against the real ROM
  (POKEMON_RED_ROM set). This step deliberately did not.
- Expected hard spots, in order: (1) the Route 2 south-edge crossing is
  now open (EVENT_GOT_POKEDEX set in post_errand) — the old "text box
  interrupted movement at (19,9)" failure mode should be gone; if it
  returns, the fixture lost the pokedex event, check state first.
  (2) Viridian Forest grass throws wild battles — that is what the
  budget-20 Travel is for. (3) trainer ambushes on Route 2 set
  wJoyIgnore; wIsInBattle can read stale 0xff right after a battle ends —
  DecodeBattle returns nil then, do not treat 0xff as an error.
- If Travel stops short of the Place: it settles on the leg's landing
  tile, so the EXACT-arrival assertion is the real test of whether
  Travel's walkWithinMap reaches (14,8). If it fails there, measure the
  actual stop point from the Fatalf before changing anything.
- Route recap: Viridian north edge (row 0, x17-19) -> 0x0D at (8,71) ->
  forest -> Route 2 north -> Pewter (0x02).
- Do not add/rename/bump fixtures unless the test demands a new
  checkpoint; if one is added, bump fixtureVersion (cache is v5 now).
