# RUNNOTES — S5c-2: re-plan exhaustion now unambiguously terminal

## For the next task (S5c-6: prove the badge)
- S5c-2 changed the ERROR TYPE ONLY: re-plan loop, maxReplans=8, and per-tile ban keying (legAt{e,m,x,y}) unchanged; no frame budget, walk, battle, or seed path touched — no new rDIV reseeding variable.
- skill/goto.go: the `replans > maxReplans` branch now returns newReplanExhaustedError(...), wrapping BOTH ErrReplanExhausted (new exported terminal sentinel) AND the last leg's error (usually ErrLegUnwalkable) via two %w verbs (Go 1.20+).
- skill/goto_replan_test.go asserts BOTH errors.Is checks; pure, fast, no ROM. Written first, watched to fail compile before the helper.
- Verified: `go test ./...` with POKEMON_RED_ROM, -skip TestGymBoulderBadge: 159 pass, 0 fail, 0 skip (TestProbe's env-gated skip is the permanent probe harness, pre-existing). TestWalkAroundGivesUpAfterMaxRetries passes unchanged.
- S5c-6: TestGymBoulderBadge is the gate to prove (excluded here on purpose). If it still loses to Brock, the S5b-6 finding likely matters first: Route 2's ledge split needs a world/-scoped fix (bounded start-map revisit in FindRouteAvoiding, or per-band map nodes) before the forest route is plannable. The preserved TestTravelToPewter below is the travel-milestone check to restore once world/ is fixed.

## Preserved TestTravelToPewter (verbatim — drop into skill/travel_test.go, plus the red/state import)
```go
func TestTravelToPewter(t *testing.T) {
	e := fixture.Load(t, "post_errand")
	dest, ok := skill.Place("pewter city")
	if !ok {
		t.Fatal(`Place: "pewter city" not found`)
	}
	res, err := skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 20)
	if err != nil {
		t.Fatalf("Travel to pewter city: %v; stopped on map %#04x at (%d,%d) after %d battles",
			err, e.Peek8(sym.CurMap), e.Peek8(sym.XCoord), e.Peek8(sym.YCoord), res.Battles)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	p := state.DecodePlayer(&mem)
	if p.MapID != dest.Map || p.X != dest.X || p.Y != dest.Y {
		t.Fatalf("player at (map %#04x, %d, %d), want Place(pewter city) = (map %#04x, %d, %d); Battles=%d BlackedOut=%v",
			p.MapID, p.X, p.Y, dest.Map, dest.X, dest.Y, res.Battles, res.BlackedOut)
	}
	if !state.Controllable(&mem) {
		t.Fatalf("player not controllable at Pewter; Battles=%d BlackedOut=%v",
			res.Battles, res.BlackedOut)
	}
	t.Logf("reached Pewter City at Place(%s) after %d battles (BlackedOut=%v)", "pewter city", res.Battles, res.BlackedOut)
}
```
