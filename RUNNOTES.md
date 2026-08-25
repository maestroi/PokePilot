# RUNNOTES — S3-2 (done)

## What changed
Commit "skill: endure a scripted cutscene without fighting the game".

- `skill/cutscene.go` (new): `Cutscene(m, budgetFrames, done func(*state.Mem) bool)`.
  Loops until `done()` AND `state.Controllable` hold, or the frame budget is spent.
  Each iteration: if `sym.FontLoaded != 0` tap A (hold 3, gap 7 = 10 frames) to
  advance the box; otherwise step one frame and wait. NEVER presses a direction.
  On timeout returns an error wrapping `ErrCutsceneTimeout` that names the map,
  (x,y), wJoyIgnore and wFontLoaded — diagnosable without a screenshot.
- `skill/cutscene_test.go` (new, ROM-gated): boots, GoTo Pallet Town, walks the
  verified path [R R R U U U U R R U] to (10,1), waits for wJoyIgnore != 0,
  confirms the up step is blocked, runs Cutscene with done=HasEvent(
  EventFollowedOakIntoLab), then asserts the flag flipped, wJoyIgnore back to 0,
  and Controllable.

## Why / measured result
This is the measurement that confirms S3-1's derived bit index 0 for
EVENT_FOLLOWED_OAK_INTO_LAB. On the real ROM: gate fired at (10,1) with
wJoyIgnore=0xFC (SELECT|START|dpad), the step into the exit was blocked, and
after the cutscene the flag flipped, wJoyIgnore returned to 0, and the player is
controllable (in Oak's lab, map 0x28). Bit index 0 is CONFIRMED.

## Must know for next task
- The Oak cutscene is long: "HEY WAIT!" box -> Oak walks over -> "not safe" box ->
  player is DRAGGED across the map boundary into the lab -> walks up 8 tiles ->
  OaksLabFollowedOakScript sets EVENT_FOLLOWED_OAK_INTO_LAB (+ _2) -> Oak's
  choose-mon speech. Cutscene returns only after that speech closes (Controllable).
  A 30000-frame budget is plenty; the drag is a simulated joypad walk the game
  drives, so no direction input is needed (or allowed).
- Pallet Town is 20x18 tiles; the north exit (to Route 1, map 0x0c) is at
  (10,1)/(11,1) — the only walkable tiles on row y=1. The gate script
  (PalletTownDefaultScript) fires at y==1.
- world/, red/, emu/ untouched. TestGoToViridianPokecenter still red (by design,
  until S3-7); verify with `-skip TestGoToViridianPokecenter`.
- ROM: /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb
