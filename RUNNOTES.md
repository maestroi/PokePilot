# RUNNOTES — S5b-3b verify: full suite green, zero skips

## What changed
No code. The first edit (conditional restore) was a no-op: the tree
was clean at HEAD (22822c3) for skill/errand_test.go,
skill/fixture/fixture.go, and RUNNOTES.md, so nothing was restored.
This note is the only file changed: task 7's commit (22822c3)
overwrote a69ec47's note and dropped its "Measured" section, which
S5b-3b's acceptance criteria require, so it is carried here.

## Verification (this run)
- go build ./... && go vet ./...: clean (exit 0).
- go test -v ./... with POKEMON_RED_ROM set to the local gomeboy ROM:
  9/9 packages ok, 0 "--- SKIP", 0 "--- FAIL", 43s wall on the warm
  v4 cache. Log: /tmp/opencode/s5b3b-verify.log.
- Source facts: TestOaksParcelOpensViridianNorthGate
  (skill/errand_test.go:190, both legs (19,10)->(19,9)->(19,8)
  asserted), TestPostErrandFixture (skill/errand_test.go:278, pins
  (19,8) on 0x01), fixtureVersion = 4 (skill/fixture/fixture.go:31).

## Measured
This verification: 43s wall, 9/9 packages ok, zero skips. Original
S5b-3b run (a69ec47): ~54s warm (skill 42s, agent 38s, fixture 1.8s);
gate tiles were verified against the static grid there.

## For the next task (cross Route 2 to Pewter, then the gym)
- Load fixture "post_errand": (19,8) on 0x01, Pokedex held, no
  parcel, controllable. fixtureVersion is 4; bump only if the
  definition of a valid state changes again.
- The Route 2 exit is the city's north edge (row 0, x17-19); the
  landing on Route 2 (0x0D) is (8,71), Place("route 2").
- Do NOT route through Route 22 (rival stands there; the forced
  battle aborts Travel) — the city's west edge is the R22 edge.
- Tall grass: Travel, never GoTo (GoTo aborts on wild battles by
  design). Route 2 is 20x72, the forest 34x48 — do not hand-verify
  whole grids; that burned three S5-3 attempts.
- BrockData: `db $FF, 12, GEODUDE, 14, ONIX, 0`. Fixture Squirtle
  learns BUBBLE at 8 and walks both. Do not tune the move policy for
  Charmander — unwinnable there, documented, not our problem.
