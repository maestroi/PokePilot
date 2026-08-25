# RUNNOTES — R2 story flags (done)

## What changed
Commit "state: fix the event index count, and read the ball's status flag".

- `red/state/progress.go`: EventOakAppearedInPallet 38 -> 39 (the count
  dropped EVENT_GOT_POKEDEX and EVENT_PALLET_AFTER_GETTING_POKEBALLS_2).
  Added EventGotPokedex = 37. Const block now documents the
  const/const_skip counting rule. Added `TookStarterBall(m *Mem) bool`
  = wStatusFlags4 bit 3.
- `red/sym/addresses.go`: StatusFlags4 = 0xD72E (pokered symbol
  wStatusFlags4).
- `red/sym/addresses_test.go`: wStatusFlags4 added to the pokered.sym
  table — it passes, so 0xD72E is confirmed against the symbol file.
- `red/state/progress_test.go`: new tests — bit 7 of 0xD74B sets
  EventOakAppearedInPallet (bit 6 does not; that was the shipped
  off-by-one), EventGotPokedex is bit 5 of the same byte, and
  TookStarterBall (0xD72E bit 3) is independent of EventGotStarter
  (0xD74B bit 2) in both directions.

## Verified
- Build, vet, full suite green with `go test ./... -skip
  TestGoToViridianPokecenter` (stays red until the plan's last task).
- Re-counted event_constants.asm by hand: 32..39 matches the task's
  table. Five indices S3-2..S3-5 use (0, 33, 34, 35, 36) unchanged.

## Must know for next task (R3)
- TWO flags mean "got a starter": TookStarterBall (wStatusFlags4 bit 3,
  0xD72E) is set the moment the player takes a ball (OaksLabMonChoiceMenu,
  right after AddPartyMon). EventGotStarter (wEventFlags bit 34, byte
  0xD74B bit 2) is set later in OaksLabRivalChoosesStarterScript, after
  the RIVAL takes his mon. R3 must use TookStarterBall for the player's
  choice and EventGotStarter for the rival's.
- wStatusFlags4 other bits (ram_constants.asm): 0 GOT_LAPRAS, 2
  USED_POKECENTER, 4 NO_BATTLES, 5 BATTLE_OVER_OR_BLACKOUT, 6
  LINK_CONNECTED.
- Event 38 (EVENT_PALLET_AFTER_GETTING_POKEBALLS_2) is deliberately
  unnamed; it renders as unknown(38).
- skill/, world/, emu/ untouched.
