# RUNNOTES — S5b-3a redo: parcel delivery (OaksParcel)

## Verdict: the task's ball premise is a wrong assumption (third time)
The hand-over chain (OaksLab.asm .got_parcel 1011 -> RIVAL_ARRIVES 510 ->
OAK_GIVES_POKEDEX 554 -> RIVAL_LEAVES 628 -> Noop) ends with the Pokedex
handed over and the rival spawned on Route 22 (both R22 events set, 636-638).
It does NOT give 5x POKE_BALL or set EVENT_GOT_POKEBALLS_FROM_OAK:
.give_poke_balls (1022-1029) is a separate, later talk with Oak, gated on
EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE (988-989), whose only setter is
Route22.asm:167 (the R22 rival after-battle script). The prior run measured
the same on the real ROM; no RAM writes or forced flags made the premise
look like it held.

## What changed
- skill/errand.go: kept the previously-measured body (GetParcel, Travel back,
  Face Oak, tap A, advanceUntil on GOT_POKEDEX&&Controllable); fixed the doc
  comment (true chain terminus, ball-gate evidence); destination now goes
  through Place("oak's lab").
- skill/goto.go: new place "oak's lab" 0x28 (5,3) — the open floor below Oak
  (5,2); the tile GetStarter's cutscene leaves; no NPC home tile.
  TestPlaceDestinationsStandable covers it automatically.
- skill/errand_test.go: TestOaksParcel now passes: hard-asserts (1) Pokedex
  set + parcel consumed + chain terminus (lab, controllable — wJoyIgnore is
  cleared only by RIVAL_LEAVES), and pins (2) flag unset and (3) 0 balls as
  measured ROM behavior; the prior version failed deliberately and left the
  suite red, which triggered this recovery — a finding should be pinned.

## Measured
TestOaksParcel 8.61s; TestGetParcel 2.50s; skill package 31.7s; whole suite
~72s; zero skips. After delivery: GOT_POKEDEX set, GOT_POKEBALLS_FROM_OAK
unset, no parcel and no balls in the bag, player at (5,3) on 0x28 controllable.

## For the next task (S5b-3b: walk the Viridian gate, register fixture)
- GOT_POKEDEX is set, so the sleeping-old-man gate at Viridian (19,9) is open — that is the walk.
- The rival is now waiting on Route 22 (both R22 events set). Do not route
  any test through Route 22: the forced second battle aborts Travel.
  Viridian->Pewter does not cross it.
- The fixture you register is captured after the gate walk; bump
  fixtureVersion when adding it (skill/fixture/fixture.go, currently 3).
