# RUNNOTES — S2-0 player facing decoder fix (DONE, commit a846097)

## What I changed
- `red/sym/addresses.go`: added `SpritePlayerFacing uint16 = 0xC109`
  (wSpritePlayerStateData1 + 9) in the Player/world const block.
- `red/state/player.go`: `DecodePlayer` now reads Facing from
  `sym.SpritePlayerFacing` instead of `sym.PlayerDirection`.
  FacingDown/Up/Left/Right constants (0/4/8/12) unchanged — they were
  always correct; only the address was wrong.
- `red/state/player_test.go`: TestDecodePlayer pokes
  `sym.SpritePlayerFacing` (value 4) instead of `sym.PlayerDirection`.
- `red/sym/addresses_test.go`: asserts `SpritePlayerFacing ==
  sym["wSpritePlayerStateData1"] + 9`. The .sym file has no plain
  "facing" symbol the pairs table can use at +9, so this is a dedicated
  base+9 check (the pairs table asserts exact symbol==constant equality).

## Why
0xD52A (wPlayerDirection) is a BITMASK (RIGHT=1 LEFT=2 DOWN=4 UP=8);
0xC109 holds SPRITE_FACING_* (DOWN=0 UP=4 LEFT=8 RIGHT=12). Phase 1
compared the bitmask against 0/4/8/12, so e.g. facing right (bitmask 1)
decoded as "unknown(1)".

## Verification
- `go build ./...`, `go vet ./...` clean.
- `POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb go test -count=1 ./...` all pass (emu, red/rom, red/state, red/sym, skill, skill/fixture, world).
- TestAddressesMatchSymbolFile and TestDecodePlayer* confirmed PASS in verbose (not skipped; /home/maestro/.cache/pokered/pokered.sym present).

## Notes for next task
- .sym file: `00:c100 wSpritePlayerStateData1`; the facing byte also has
  its own symbol `00:c109 wSpritePlayerStateData1FacingDirection` if a
  later task wants an exact-name lookup.
- wPlayerDirection (0xD52A) and wPlayerMovingDirection (0xD528) remain
  defined in sym for the bitmask use case; nothing else in the repo reads
  PlayerDirection now (only docs mention it).
- emu/, skill/, world/ untouched, per task scope.
- ROM path for tests: /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb
  (absolute; relative paths fail from subpackage dirs).
- Fixture cache at skill/testdata/fixtures/ (gitignored) still valid.
