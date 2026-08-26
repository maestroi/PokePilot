# RUNNOTES — S5b-3a: deliver Oak's parcel to the lab
## Verdict: BUILT ON A WRONG ASSUMPTION
Delivering the parcel was expected to make Oak give 5x POKE_BALL
(EventGotPokeballsFromOak=36). The ROM does not: the only code that sets that
flag or gives the balls is `.give_poke_balls` (pokered/scripts/OaksLab.asm:
1022-1025), reached only when EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE is set
(OaksLab.asm:988-989), set in exactly one place: Route22.asm:167 (a Route 22
rival battle, many towns later). The parcel chain never sets it; a whole-tree
grep confirms no other setter. So postconditions (2)/(3) are unreachable from
post_starter+parcel; only (1) Pokedex is satisfied. No hack applied.

## What changed
- skill/errand.go: `OaksParcel(m, romData, policy) error`. Calls GetParcel,
  Travel to lab 0x28 at (5,3) (tile below Oak(5,2)); a Travel failing ON the
  lab is the normal entry (door force-walks) -> Cutscene(labEntryBudget,
  Controllable) -> re-Travel. Then Face(5,2), Tap A, advanceUntil
  (deliveryBudget=40000) until EventGotPokedex && Controllable. Exports
  ItemPokeBall=0x04; reuses oaksLabMap/Place/Destination/Travel/Cutscene.
- skill/errand_test.go: TestOaksParcel (post_starter). Hard-asserts the
  reachable results (Pokedex set, parcel consumed, controllable, in lab); for
  (2)/(3) it checks them and FAILS with a "WRONG ASSUMPTION" diagnostic
  carrying file:line evidence (intentionally not weakened to pass).

## How the hand-over works (decomp)
A TALK, not a map script: OaksLabOak1Text `.check_got_parcel`(1004) ->
`.got_parcel`(1011): DeliverParcelText, RemoveParcel, sets
wOaksLabCurScript=RIVAL_ARRIVES(510) -> OAK_GIVES_POKEDEX(554: sets
GOT_POKEDEX=37 + OAK_GOT_PARCEL=56) -> RIVAL_LEAVES(624, hides rival, NOOP).
Talk is unusable (its controllable-wait breaks on JoyIgnore), so advanceUntil drives it.

## Measured (POKEMON_RED_ROM=.../gomeboy/roms/pokemon_red.gb)
TestOaksParcel 8.7s: GOT_POKEDEX=true, GOT_POKEBALLS_FROM_OAK=false,
GOT_OAKS_PARCEL=true, BAG=[] (0 pokeballs), (5,3) on 0x28 controllable ->
fails WRONG ASSUMPTION (expected). TestGetParcel PASS (~2.5s); suite ok else; 0 skips.

## For the next task
Lab is 0x28 (the old S5b-2 note's "(0x0A)" was a typo); Oak1 (5,2), approach
(5,3). Earning Oak's 5x POKE_BALL needs beating the Route 22 rival first
(Route22.asm:167) — far outside errand scope; do not scope it here.
