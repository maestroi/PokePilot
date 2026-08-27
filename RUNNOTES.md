# RUNNOTES — S5b-3b: open the Viridian north gate (post_errand fixture)

## Verdict: gate open; fixture post_errand registered and cached at v4.
TestOaksParcelOpensViridianNorthGate: the errand (OaksParcel) sets
EVENT_GOT_POKEDEX; Travel to (23,26) on 0x01; GoTo (19,10), the tile
south of the gate line; StepOnce up to (19,9) — free even with the gate
closed, so not the assertion; StepOnce up to (19,8) — the step the
closed gate refuses (box "You can't go through here!", push back to
(19,10)). Crossing 10.6s from the warm post_starter fixture.
TestPostErrandFixture forces the new fixture's build and pins the
resulting state: (19,8) on 0x01, Pokedex held, parcel consumed,
controllable. Loaded from cache in 0.00s on later runs.

## What changed
- skill/errand_test.go: +TestOaksParcelOpensViridianNorthGate (both
  crossing legs asserted, since reaching (19,9) alone proves nothing),
  +TestPostErrandFixture (builds the fixture, pins its postconditions).
- skill/fixture/fixture.go: +Register("post_errand"): starter ->
  OaksParcel -> Travel "viridian city" (23,26) -> GoTo (19,10) ->
  StepOnce up x2 -> (19,8). fixtureVersion 3 -> 4, per the S5b-3a
  handoff: every v3 cache invalidated and rebuilt on this run.

## Measured
Whole suite: 9 packages ok, zero skips, ~54s wall on the warm v4 cache
(skill 42s, agent 38s, fixture 1.8s). Gate tiles verified against the
static grid in a scratch program first: (19,8)/(19,9)/(19,10) all
walkable, no warps; the approach (23,26)->(19,10) is 20 monotone steps
that stay south of the gate, and the default map script is inert with
the flag set (gym check only fires at (32,8)).

## For the next task
- Load fixture "post_errand": (19,8) on 0x01, Pokedex held, no parcel.
- The Route 2 exit is the city's north edge (row 0, x17-19); the
  landing on Route 2 (0x0D) is (8,71), Place("route 2").
- Do not route through Route 22 (rival stands there; the forced
  battle aborts Travel) — the city's WEST edge is the R22 edge.
- fixtureVersion is 4; bump it only if the definition of a valid
  state changes again.
