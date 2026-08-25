# RUNNOTES — R5 GetStarter (done, ROM-gated test skipped locally)

## What changed
Commit "skill: follow Oak, take a starter, beat the rival".

- `skill/story.go` (new): `GetStarter(m, romData, which Starter, policy
  MovePolicy) error`. Flow: GoTo Pallet Town -> WalkPath right3/up4/right2/up
  to (10,1) (gate fires, wJoyIgnore != 0) -> Cutscene on
  EventOakAskedToChooseMon -> controllable at lab (5,3) -> walkLab to the
  approach tile below the chosen ball -> Face(ball) -> A -> wait for the
  YesNoChoice shape (FontLoaded != 0 && DecodeMenu().Max == 1) ->
  SelectMenuItem(0) YES -> wait TookStarterBall && party >= 1 -> Cutscene on
  EventGotStarter -> walkLab to row 6 (challenge text = expected
  ErrDialogueInterrupted) -> advanceUntil DecodeBattle != nil ->
  Battle(m, policy), require ResultWon -> wait EventBattledRivalInOaksLab,
  assert Controllable. Idempotent: nil if that event is already set.
- `skill/story_test.go` (new): ROM-gated. GetStarter with StarterSquirtle +
  StatAwareMove, second call for idempotency, then GoTo Pallet Town ->
  gatePath -> HoldUntil(Up, 300, CurMap != 0x00) -> assert CurMap == 0x0C.

## Verified
- Build, vet, `go test ./... -skip TestGoToViridianPokecenter` all green;
  TestGetStarter SKIPs here (POKEMON_RED_ROM not set).
- world/, red/, emu/, skill/battle.go, skill/policy.go untouched.

## Gotchas baked in (measured facts, do not re-derive)
- Yes/no menu shape is Max == 1 (highest valid index, inclusive); Start
  menu Max == 6. SelectMenuItem(0) = YES under either reading.
  wMaxMenuItem is stale-0 at boot, so Max == 1 identifies the choice box.
- During any battle wFontLoaded == 0. Controllable = CurMapWidth/Height !=
  0, FontLoaded == 0, JoyIgnore == 0, WalkCounter == 0.
- Lab dynamic obstacles (rival (4,3), Oak (5,2), balls (6,3)-(8,3)) are not
  in the static grid; walkLab re-plans around ErrBlocked. Challenge text
  opens on row 6; accept ErrDialogueInterrupted at any row-6 tile.
- Budgets (frames): gate 60, cutscene 30000, choice 3000, starter 10000,
  battle 10000. Whole story ~8 s of game time.

## Next task
- Call skill.GetStarter from a fresh fixture; Route 1 is the north edge at (10,0).
