# RUNNOTES — Travel: fight wild encounters, resume the route (done)

## What changed
Commit "skill: fight through wild encounters and resume the route".

- `skill/travel.go` (new): `Travel(m, romData, dest, policy, maxBattles)
  (TravelResult, error)`. Loop: GoTo -> on nil, done; on non-ErrBattle,
  return unchanged; on ErrBattle, Battle(m, policy) and continue.
  ResultLost sets BlackedOut and continues (GoTo re-plans from the Center);
  it is not an error. Battle error is returned wrapped with the battle
  count. maxBattles <= 0 is an error before anything walks; after
  maxBattles battles the loop returns an error plus the result so far.
- `skill/travel_test.go` (new): ROM-gated (skips without POKEMON_RED_ROM).
  Pallet Town grass-free: Battles 0, BlackedOut false. maxBattles 0: error
  mentioning the bound, position unchanged. Nonsense dest (map 0xFF, above
  maxMapID 0xF7 so never a graph node): error comes back unchanged,
  errors.Is(err, world.ErrNoRoute) true, NOT ErrBattle. Pallet -> Viridian
  (maxBattles 20): wCurMap == 0x01 and Battles >= 1.

## Verified
- Build, vet, `go test ./... -skip TestGoToViridianPokecenter` green with
  POKEMON_RED_ROM set; all four Travel tests SKIP cleanly without it.
- goto.go, move.go, warp.go, battle.go untouched (empty diff).
- Viridian result, verbatim: "reached Viridian City after 1 battles
  (BlackedOut=false)" — PASS in ~9.4 s. The measured GoTo stall
  ("battle on map 0c at (14,7)") is fought and the route finishes.

## Gotchas baked in (do not re-derive)
- errors.Is(err, ErrBattle) is the single battle check: 41e0cca normalized
  both walkWithinMap and Traverse to wrap ErrBattle. Travel must NOT
  re-wrap or convert non-battle errors; the nonsense-dest test guards that.
- Map 0xFF is a safe "no route" destination: BuildGraph only parses
  0..0xF7, so it is never a node and no edge ever has To == 0xFF.
- The battle at (14,7) on Route 1 is the only one on this route in a
  measured run (Battles = 1); the 20 bound is headroom, not expectation.
- Squirtle + StatAwareMove wins every Route 1 wild (Pidgey/Rattata lvl 2-3).

## Next task
- R6 (plan ff1c6b79) can now resume: its TestGoToViridianPokecenter crosses
  Route 1's grass and needs this plus its fixture work. Still skipped here.
- Do not point Travel at "viridian pokemon center" yet; that is R6's call.
