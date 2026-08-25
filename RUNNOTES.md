# RUNNOTES — S3-3 (done)

## What changed
Commit "skill: cursor-driven menu selection".

- `red/state/menu.go` (new): `MenuState{Current int, Max int}`, `DecodeMenu(m *Mem) MenuState`
  (value, reads wCurrentMenuItem/wMaxMenuItem, no gating).
- `skill/menu.go` (new): `ErrMenuStuck`, `SelectMenuItem(m, index)`. Step-and-verify: tap toward
  target, re-read wCurrentMenuItem, repeat until asserted == index, then tap A. Range check
  `index < 0 || index >= menu.Max`. 5 consecutive stalled taps -> ErrMenuStuck.
- `red/state/menu_test.go` (new, synthetic Mem), `skill/menu_test.go` (new, ROM-gated Start menu).
- Reconciled the pre-existing conflicting `MenuState`/`DecodeMenu` in `red/state/ui.go` (was
  uint8, returned *MenuState, gated on FontLoaded): removed it; `state.go` `Menu *MenuState` ->
  value; `boot.go` menu-open check now uses `FontLoaded != 0`; `ui_test.go` menu tests dropped.
- `red/sym/addresses.go`: added `ListMenuID` (0xCF94).

## Why / measured ground truth (verified on the ROM — corrects the task spec)
- `wMaxMenuItem` is the item COUNT, not an inclusive max: Start menu (no pokedex) = 6, valid
  indices 0..5. The cursor WRAPS (5->0 on Down), contrary to the "does not wrap" assumption.
  Hence the range check is `index >= Max` (chasing index==Max would loop forever: it wraps and
  never reads as stuck).
- A on Start-menu index 1 (ITEM) opens the bag, identified by `wListMenuID == 3` (ITEMLISTMENU);
  index 0 (POKéMON) opens the party submenu, which is NOT a list menu (listID stays 0).
- The fixture bag is EMPTY: it reports `{Current:0 Max:0}` (list max = count when <2), not `{Max:1}`.
- Menu RAM holds stale values from boot (cur=5, max=7). FontLoaded fires BEFORE DrawStartMenu
  writes the cursor, so "menu fully drawn" = `wMaxMenuItem == 6` (the last write in DrawStartMenu).
- The list menu's input path needs the joypad to settle after the A press: close with B using a
  20-frame settle + hold 8 (a plain hold-3 tap right after A is missed).

## Must know for next task
- Start-menu open predicate: tap Start, wait `FontLoaded != 0` AND `wMaxMenuItem == 6`.
- Select index i: `skill.SelectMenuItem(e, i)`; it asserts the cursor reached i before A.
- Distinguish submenus by wListMenuID (3 = bag), not by wMaxMenuItem alone (party also reads 0).
- world/, emu/, battle code untouched. TestGoToViridianPokecenter still red (by design, until
  S3-7); verify with `-skip TestGoToViridianPokecenter`.
- ROM: /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb
