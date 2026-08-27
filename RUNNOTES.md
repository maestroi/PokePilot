# RUNNOTES — S5b-4: heal the party at the Viridian Center nurse

## What changed
- skill/heal.go: Heal(m). The player stands on the counter
  approach tile (Place("viridian pokemon center")); the nurse
  is two tiles away, beyond Face's one-tile reach, so Heal
  faces the counter — the unique non-walkable neighbor of the
  player's tile in the ROM's collision grid — because
  overworld.asm's talk range extends over a counter in front
  of the player. It opens the dialogue (A + the Talk-style
  open wait), advances the welcome box and the first-visit
  "Shall we heal" box with advanceUntil, answers YES with
  SelectMenuItem(0) (the same two-item shape as the starter
  YesNoChoice), then Cutscene runs the YES path through
  HealParty (pokered/engine/events/heal_party.asm: HP<-max per
  mon). Postcondition: every mon HP==MaxHP and controllable;
  idempotent for a party already at full.
- skill/heal_test.go: TestHeal from the viridian_pokecenter
  fixture. Pre-asserts the party is damaged (never vacuous),
  post-asserts full HP, unchanged party size, controllable,
  position unchanged.
- No new sym addresses; fixture version stays v4; no
  coordinate literals in the skill (Place only).

## Verification (this run)
- POKEMON_RED_ROM set to the local gomeboy ROM: go build ./...
  && go vet ./... && go test ./... -count=1 -> 9/9 packages ok,
  exit 0 (/tmp/opencode/s5b4-verify.log).
- go test -v ./... -count=1 -> 0 "--- SKIP", 0 "--- FAIL"
  (/tmp/opencode/s5b4-verify-v.log).
- TestHeal passed on the first run: 554ms wall for the whole
  dialogue-plus-heal.

## Measured (committed v4 fixture)
- Arrives damaged: Squirtle (0xB1) lvl 6, HP 13/22, status 0
  - the builder's Route 1 wild battle left it below max, so
  the pre-assertion is satisfiable as-is; no Travel-based
  damage step was needed.
- BIT_USED_POKECENTER (wStatusFlags4 bit 2) unset: first
  visit, so the "Shall we heal" box appears before the
  prompt; the flow handles repeat visits (one fewer box) too.
- From (3,3) on 0x29 the only non-walkable in-bounds neighbor
  is up, (3,2) = the counter; the nurse is at (3,1).

## For the next task (cross Route 2 to Pewter, then the gym)
- Previous guidance stands: load post_errand ((19,8) on
  0x01); the Route 2 exit is the city's north edge (row 0,
  x17-19) landing (8,71); avoid Route 22 (rival stands
  there); Travel, never GoTo, in tall grass; do not
  hand-verify whole grids.
- Heal is scoped to the counter-approach contract (one
  unique solid neighbor). Centers where the nurse is
  adjacent with no counter between player and nurse (e.g.
  Pewter) fail counterDirection with a clear error; face
  those by sprite position or add an explicit approach tile
  per center.
