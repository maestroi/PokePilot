package skill

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

type fakeFieldPathGrid struct {
	w, h      int
	walkable  map[[2]int]bool
	fieldTile map[[2]int]uint8
	tile      map[[2]int]uint8
}

func newFakeFieldPathGrid(w, h int) *fakeFieldPathGrid {
	return &fakeFieldPathGrid{
		w:         w,
		h:         h,
		walkable:  map[[2]int]bool{},
		fieldTile: map[[2]int]uint8{},
		tile:      map[[2]int]uint8{},
	}
}

func (g *fakeFieldPathGrid) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.w && y < g.h
}

func (g *fakeFieldPathGrid) Walkable(x, y int) bool {
	return g.InBounds(x, y) && g.walkable[[2]int{x, y}]
}

func (g *fakeFieldPathGrid) Movement(x, y int, input world.Step, blocked map[[2]int]bool) (world.Step, bool) {
	nx, ny := x+input.DX, y+input.DY
	if blocked[[2]int{nx, ny}] || !g.Walkable(nx, ny) {
		return world.Step{}, false
	}
	return input, true
}

func (g *fakeFieldPathGrid) Tile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) {
		return 0, false
	}
	return g.tile[[2]int{x, y}], true
}

func (g *fakeFieldPathGrid) FieldTile(x, y int) (uint8, bool) {
	if !g.InBounds(x, y) {
		return 0, false
	}
	return g.fieldTile[[2]int{x, y}], true
}

func openCells(g *fakeFieldPathGrid, cells ...[2]int) {
	for _, c := range cells {
		g.walkable[c] = true
	}
}

func countFieldActions(plan []fieldPathStep, action fieldPathAction) int {
	n := 0
	for _, step := range plan {
		if step.Action == action {
			n++
		}
	}
	return n
}

func TestPlanFieldPathOpensGymCutChamberToDoorApproach(t *testing.T) {
	// Celadon Gym's leader pocket is a Cut-tree chamber: land-only FindPath
	// reports no route to the south door even when Cut is usable. Local field
	// pathing must treat the gym tree tile as a destination-aware Cut step.
	land := newFakeFieldPathGrid(5, 6)
	water := newFakeFieldPathGrid(5, 6)

	openCells(land,
		[2]int{2, 1}, [2]int{2, 2}, // sealed chamber
		[2]int{2, 4}, [2]int{2, 5}, // corridor to the door approach
	)
	for y := 0; y < 6; y++ {
		openCells(water, [2]int{2, y})
	}
	land.fieldTile[[2]int{2, 3}] = gymCutTreeTile

	plan, err := planFieldPath(land, water, gymTileset, 2, 1, 2, 5, nil, true, false, false)
	if err != nil {
		t.Fatalf("planFieldPath through gym Cut chamber: %v", err)
	}
	if got := countFieldActions(plan, fieldPathCut); got != 1 {
		t.Fatalf("Cut actions = %d, want 1; plan=%+v", got, plan)
	}
}

func TestPlanFieldPathCutsTreeOnSelectedRouteNotNearbyDeadEnd(t *testing.T) {
	land := newFakeFieldPathGrid(7, 3)
	water := newFakeFieldPathGrid(7, 3)

	for x := 0; x < 7; x++ {
		if x != 3 {
			openCells(land, [2]int{x, 1})
		}
		openCells(water, [2]int{x, 1})
	}
	// A tempting nearby tree leads only into a dead-end spur. The required
	// tree is the one that actually joins start to destination.
	openCells(land, [2]int{0, 0})
	land.fieldTile[[2]int{1, 0}] = cutTreeTile
	land.fieldTile[[2]int{3, 1}] = cutTreeTile

	plan, err := planFieldPath(land, water, overworldTileset, 0, 1, 6, 1, nil, true, false, false)
	if err != nil {
		t.Fatalf("planFieldPath: %v", err)
	}
	if got := countFieldActions(plan, fieldPathCut); got != 1 {
		t.Fatalf("Cut actions = %d, want exactly 1; plan=%+v", got, plan)
	}

	x, y := 0, 1
	cutAt := [2]int{-1, -1}
	for _, step := range plan {
		x += step.Move.DX
		y += step.Move.DY
		if step.Action == fieldPathCut {
			cutAt = [2]int{x, y}
		}
	}
	if cutAt != [2]int{3, 1} {
		t.Fatalf("planned Cut at %v, want route tree (3,1)", cutAt)
	}
}

func TestPlanFieldPathPrefersOrdinaryDetourOverUnneededCut(t *testing.T) {
	land := newFakeFieldPathGrid(5, 3)
	water := newFakeFieldPathGrid(5, 3)

	// Direct row contains a Cut tree, but the upper row is an ordinary
	// walkable detour. Field actions are intentionally secondary to walking.
	for x := 0; x < 5; x++ {
		openCells(land, [2]int{x, 0})
		openCells(water, [2]int{x, 0})
	}
	openCells(land, [2]int{0, 1}, [2]int{1, 1}, [2]int{3, 1}, [2]int{4, 1})
	openCells(water, [2]int{0, 1}, [2]int{1, 1}, [2]int{3, 1}, [2]int{4, 1})
	land.fieldTile[[2]int{2, 1}] = cutTreeTile

	plan, err := planFieldPath(land, water, overworldTileset, 0, 1, 4, 1, nil, true, false, false)
	if err != nil {
		t.Fatalf("planFieldPath: %v", err)
	}
	if got := countFieldActions(plan, fieldPathCut); got != 0 {
		t.Fatalf("Cut actions = %d, want 0 when ordinary route exists; plan=%+v", got, plan)
	}
}

func TestPlanFieldPathFastestUsesCutWhenItBeatsLongDetour(t *testing.T) {
	land := newFakeFieldPathGrid(3, 7)
	water := newFakeFieldPathGrid(3, 7)

	// Direct route is two tiles with one Cut. The zero-action alternative is a
	// long U-shaped corridor: six down, two across, six up. Conservative mode
	// must keep the detour; fastest mode prices Cut at eight movement units and
	// therefore takes the 10-unit shortcut instead of the 14-unit walk.
	for y := 0; y < 7; y++ {
		openCells(land, [2]int{0, y}, [2]int{2, y})
		openCells(water, [2]int{0, y}, [2]int{2, y})
	}
	for x := 0; x < 3; x++ {
		openCells(land, [2]int{x, 6})
		openCells(water, [2]int{x, 6})
	}
	land.fieldTile[[2]int{1, 0}] = cutTreeTile

	conservative, err := planFieldPath(land, water, overworldTileset, 0, 0, 2, 0, nil, true, false, false)
	if err != nil {
		t.Fatalf("conservative planFieldPath: %v", err)
	}
	if got := countFieldActions(conservative, fieldPathCut); got != 0 {
		t.Fatalf("conservative Cut actions = %d, want 0; plan=%+v", got, conservative)
	}

	fastest, cost, err := planFieldPathWithCost(
		land, water, overworldTileset,
		0, 0, 2, 0, nil,
		true, false, false,
		fastestFieldPathCostPolicy(),
	)
	if err != nil {
		t.Fatalf("fastest planFieldPathWithCost: %v", err)
	}
	if got := countFieldActions(fastest, fieldPathCut); got != 1 {
		t.Fatalf("fastest Cut actions = %d, want 1; plan=%+v", got, fastest)
	}
	if cost.weighted != 10 {
		t.Fatalf("fastest weighted cost = %d, want 10", cost.weighted)
	}

	withoutCut, _, err := planFieldPathWithCost(
		land, water, overworldTileset,
		0, 0, 2, 0, nil,
		false, false, false,
		fastestFieldPathCostPolicy(),
	)
	if err != nil {
		t.Fatalf("fastest plan without Cut capability: %v", err)
	}
	if got := countFieldActions(withoutCut, fieldPathCut); got != 0 {
		t.Fatalf("Cut actions without capability = %d, want 0; plan=%+v", got, withoutCut)
	}
}

func TestStrengthPlanTravelCostCanBeatLongFieldPath(t *testing.T) {
	policy := fastestFieldPathCostPolicy()
	plan := world.PushPlan{
		Pushes: []world.Push{{
			Walk: []world.Step{world.StepDown, world.StepDown, world.StepRight},
		}},
		FinalWalk: []world.Step{world.StepRight, world.StepUp},
	}
	if got := weightedStrengthPlanCost(plan, false, policy); got != 16 {
		t.Fatalf("inactive Strength weighted cost = %d, want 16", got)
	}
	if !strengthPlanBeatsFieldPath(fieldPathCost{weighted: 30}, plan, false, policy) {
		t.Fatal("Strength plan should beat a 30-unit field path")
	}
	if strengthPlanBeatsFieldPath(fieldPathCost{weighted: 12}, plan, false, policy) {
		t.Fatal("Strength plan should not beat a 12-unit field path")
	}
	if strengthPlanBeatsFieldPath(fieldPathCost{weighted: 30}, plan, false, conservativeFieldPathCostPolicy()) {
		t.Fatal("conservative policy must not prefer Strength as an optional shortcut")
	}
}

func TestPlanFieldPathUsesSurfOnlyWhenWaterIsRequired(t *testing.T) {
	land := newFakeFieldPathGrid(5, 1)
	water := newFakeFieldPathGrid(5, 1)

	openCells(land, [2]int{0, 0}, [2]int{4, 0})
	for x := 0; x < 5; x++ {
		openCells(water, [2]int{x, 0})
	}
	for x := 1; x <= 3; x++ {
		water.fieldTile[[2]int{x, 0}] = surfWaterTile
	}

	plan, err := planFieldPath(land, water, overworldTileset, 0, 0, 4, 0, nil, false, true, false)
	if err != nil {
		t.Fatalf("planFieldPath with Surf: %v", err)
	}
	if got := countFieldActions(plan, fieldPathSurf); got != 1 {
		t.Fatalf("Surf actions = %d, want exactly 1; plan=%+v", got, plan)
	}
	if len(plan) == 0 || plan[0].Action != fieldPathSurf {
		t.Fatalf("first step = %+v, want Surf entry", plan)
	}

	_, err = planFieldPath(land, water, overworldTileset, 0, 0, 4, 0, nil, false, false, false)
	if !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("without Surf err=%v, want world.ErrNoPath", err)
	}
}

func TestPlanFieldPathHonorsSurfEntryRule(t *testing.T) {
	land := newFakeFieldPathGrid(5, 1)
	water := newFakeFieldPathGrid(5, 1)

	openCells(land, [2]int{0, 0}, [2]int{4, 0})
	for x := 0; x < 5; x++ {
		openCells(water, [2]int{x, 0})
	}
	for x := 1; x <= 3; x++ {
		water.fieldTile[[2]int{x, 0}] = surfWaterTile
	}

	denyStart := fieldPathRules{
		SurfAllowedFrom: func(x, y int) bool {
			return x != 0 || y != 0
		},
	}
	_, err := planFieldPath(land, water, overworldTileset, 0, 0, 4, 0, nil, false, true, false, denyStart)
	if !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("blocked Surf entry err=%v, want world.ErrNoPath", err)
	}

	plan, err := planFieldPath(land, water, overworldTileset, 0, 0, 4, 0, nil, false, true, false, fieldPathRules{})
	if err != nil {
		t.Fatalf("unrestricted Surf entry: %v", err)
	}
	if got := countFieldActions(plan, fieldPathSurf); got != 1 {
		t.Fatalf("Surf actions = %d, want 1; plan=%+v", got, plan)
	}
}

func TestPlanFieldPathTreatsForcedMovementAsOneEdge(t *testing.T) {
	land := newFakeFieldPathGrid(5, 1)
	water := newFakeFieldPathGrid(5, 1)
	for x := 0; x < 5; x++ {
		openCells(land, [2]int{x, 0})
		openCells(water, [2]int{x, 0})
	}

	rules := fieldPathRules{
		ForcedLanding: func(x, y int) (world.Point, bool) {
			if x == 1 && y == 0 {
				return world.Point{X: 3, Y: 0}, true
			}
			return world.Point{}, false
		},
	}
	plan, err := planFieldPath(land, water, overworldTileset, 0, 0, 4, 0, nil, false, false, false, rules)
	if err != nil {
		t.Fatalf("planFieldPath forced movement: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("plan length = %d, want forced edge + final walk; plan=%+v", len(plan), plan)
	}
	if plan[0].Action != fieldPathForced || plan[0].Landing != (world.Point{X: 3, Y: 0}) {
		t.Fatalf("first step = %+v, want forced landing (3,0)", plan[0])
	}
	if plan[1].Action != fieldPathWalk || plan[1].Move != world.StepRight {
		t.Fatalf("second step = %+v, want ordinary right walk", plan[1])
	}
}

// TestBlockingUndefeatedTrainerSingleGate is Silph Co 5F's Card Key room
// in miniature: MEASURED against the real ROM (skill/probe_test.go's
// TestProbe), the only corridor from the 5F stair landing to the Card Key
// runs through Rocket2's tile at (28,4). currentObservedStationaryObjectBlockers
// correctly marks that tile solid, so a plain reachability probe reports a
// dead end; unblocking exactly that one tile is what should make dest
// reachable again, which is the fact this test fixes in place.
func TestBlockingUndefeatedTrainerSingleGate(t *testing.T) {
	rocket := rom.Object{X: 28, Y: 4}
	scientist := rom.Object{X: 8, Y: 3}
	reachable := map[[2]int]bool{
		{int(rocket.X), int(rocket.Y)}: true,
	}
	candidate, ok, err := blockingUndefeatedTrainer([]rom.Object{scientist, rocket}, func(at [2]int) (bool, error) {
		return reachable[at], nil
	})
	if err != nil {
		t.Fatalf("blockingUndefeatedTrainer: %v", err)
	}
	if !ok {
		t.Fatal("blockingUndefeatedTrainer: ok = false, want the single reachability-restoring trainer")
	}
	if candidate != rocket {
		t.Fatalf("candidate = %+v, want %+v", candidate, rocket)
	}
}

// TestBlockingUndefeatedTrainerDeclinesWhenAmbiguous mirrors
// currentLocalStrengthPlan's refusal to move a boulder that does not
// provably open the exact destination: if unblocking either of two
// candidate trainers alone would reopen the route, fighting the wrong one
// first wastes a battle without helping, so the probe must decline rather
// than guess.
func TestBlockingUndefeatedTrainerDeclinesWhenAmbiguous(t *testing.T) {
	a := rom.Object{X: 8, Y: 16}
	b := rom.Object{X: 28, Y: 4}
	_, ok, err := blockingUndefeatedTrainer([]rom.Object{a, b}, func(at [2]int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("blockingUndefeatedTrainer: %v", err)
	}
	if ok {
		t.Fatal("blockingUndefeatedTrainer: ok = true with two equally-reopening candidates, want false")
	}
}

// TestBlockingUndefeatedTrainerDeclinesWhenNoneHelp covers the case that
// walkWithinMap must still fall through to arriveBesideBlockedDestination
// for: an undefeated trainer sits on a blocked tile, but the destination is
// genuinely unreachable regardless (a real dead end, not a gauntlet gate).
func TestBlockingUndefeatedTrainerDeclinesWhenNoneHelp(t *testing.T) {
	a := rom.Object{X: 8, Y: 16}
	_, ok, err := blockingUndefeatedTrainer([]rom.Object{a}, func(at [2]int) (bool, error) {
		return false, nil
	})
	if err != nil {
		t.Fatalf("blockingUndefeatedTrainer: %v", err)
	}
	if ok {
		t.Fatal("blockingUndefeatedTrainer: ok = true when no candidate reopens dest, want false")
	}
}

// TestBlockingUndefeatedTrainerPropagatesError proves a plan-time error other
// than world.ErrNoPath (a malformed map, not a mere disconnection) aborts the
// probe instead of being swallowed as "this trainer does not help".
func TestBlockingUndefeatedTrainerPropagatesError(t *testing.T) {
	a := rom.Object{X: 8, Y: 16}
	wantErr := fmt.Errorf("boom")
	_, _, err := blockingUndefeatedTrainer([]rom.Object{a}, func(at [2]int) (bool, error) {
		return false, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("blockingUndefeatedTrainer error = %v, want %v", err, wantErr)
	}
}
