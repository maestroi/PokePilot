package world

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

// Teleporter pads are walkable floor bytes, but stepping on one leaves the
// map. Component flood-fill must not treat them as ordinary corridors, or
// rooms that are only joined by a pad (Silph Co 5F's Card Key hallway across
// (9,15)) collapse into one component and GoTo never leave/re-enters.
//
// Pristine ROM collision alone still connects the stair landing to the Card
// Key through the Rocket's home tile; overlaying still-present stay objects
// marks that tile occupied. Together, warp punching + object overlay make
// leave/re-enter the honest route.
//
// Sibling farm fingerprints of the same no_path (AcquireSilphCardKey /
// approach beside the Card Key, oscillation (8,15)<->(28,3) under
// sprite-fallback):
//   - run-xgoe3a12m8xdt (triage:2f31f14067d98e4a, farm-issue:1374; #1384)
//   - run-klyyfags4zp2o83zk0lvxsgx (triage:15a12d2232f2261d, farm-issue:1391;
//     observed on runner 5c5d020e before #1384 deployed)
//   - run-147obvg8zhcn32hbhtbui3m2gf (triage:e30926cf9d590d68, farm-issue:1392;
//     resume of run-77gc6gouf4ph1udc0lfs70evu on runner 242849dc before #1384)
//   - run-1xoateppursxmhuq6pgsbmupq (triage:c3710db68ddcea92, farm-issue:1377;
//     same no_path on runner before #1384; AcquireSilphCardKey from
//     round-01_start.state reproduces the stuck (28,3) path until warp
//     punching + presentStationaryObjectBlockers)
//   - run-8s9ydjiftbb510ch1h786tdj2 (triage:9d5b4f4dad3316f4, farm-issue:1375;
//     resume of run-xgoe3a12m8xdt on runner aafd8b76 before #1384; round-003
//     AcquireSilphCardKey from (8,15) oscillated to (28,3) until the same fix)
//   - run-20u6ntzbz2y1r3aokln4zvlx85 (triage:d9dc009d30d09325, farm-issue:1393;
//     observed on runner 5c5d020e before #1384; stuck at SILPH_CO_5F (28,3)
//     with GoTo no capability-aware path to (20,16))
func TestSilphCo5FWarpPadSplitsCardKeyComponent(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	const silph5F uint8 = 0xd2
	h, err := rom.ParseMap(romData, silph5F)
	if err != nil {
		t.Fatalf("ParseMap: %v", err)
	}
	grid, err := Build(romData, h)
	if err != nil {
		t.Fatalf("Build grid: %v", err)
	}
	for _, o := range h.Objects {
		if o.Movement == rom.MovementStay {
			grid.Set(int(o.X), int(o.Y), false)
		}
	}
	g2, err := g.WithMapGrid(silph5F, grid)
	if err != nil {
		t.Fatalf("WithMapGrid: %v", err)
	}
	comps := g2.comps[silph5F]
	start := comps[1][26]
	cardKeyApproach := comps[16][20]
	if start == 0 || cardKeyApproach == 0 {
		t.Fatalf("missing components: stair landing=%d card-key approach=%d", start, cardKeyApproach)
	}
	if start == cardKeyApproach {
		t.Fatalf("stair landing (26,1) and Card Key approach (20,16) share component %d after warp+object overlay", start)
	}

	route, err := FindRouteAtDestination(g2, silph5F, silph5F, 26, 1, 20, 16, nil)
	if err != nil {
		t.Fatalf("FindRouteAtDestination: %v", err)
	}
	if len(route) < 2 {
		t.Fatalf("route = %+v, want a leave/re-enter cycle through another floor", route)
	}
	if route[0].From != silph5F || route[0].To == silph5F {
		t.Fatalf("first hop = %+v, want leave Silph Co 5F", route[0])
	}
	last := route[len(route)-1]
	if last.To != silph5F {
		t.Fatalf("last hop = %+v, want re-enter Silph Co 5F", last)
	}

	// Speedrun pricing must not collapse this split back into a local walk.
	// Farm run-3uur450e1esuu2liiozd83sv9n stood at (28,3) and GoTo reported
	// no path to (20,16) because the weighted route was empty.
	weighted, werr := FindWeightedRoutePlanAtDestinationWithCapabilities(
		g2, silph5F, silph5F, 28, 3, 20, 16, nil, RoutePrerequisites{}, DefaultRouteCostPolicy(),
	)
	if werr != nil {
		t.Fatalf("weighted route from (28,3): %v", werr)
	}
	if len(weighted.Steps) == 0 {
		t.Fatalf("weighted route from (28,3) was a local walk (cost=%d exact=%v); component split must leave 5F", weighted.Cost, weighted.Exact)
	}
}

func TestComponentsWithBlockedTreatsWarpAsNonCorridor(t *testing.T) {
	// Two open cells joined only through a center "pad".
	g := &Grid{
		Width:    3,
		Height:   1,
		walkable: []bool{true, true, true},
	}
	if c := components(g); c[0][0] != c[0][2] {
		t.Fatalf("open corridor components = %v, want one region", c[0])
	}
	blocked := map[[2]int]bool{[2]int{1, 0}: true}
	c := componentsWithBlocked(g, blocked)
	if c[0][0] == 0 || c[0][2] == 0 {
		t.Fatalf("blocked-center components = %v, want both sides labeled", c[0])
	}
	if c[0][0] == c[0][2] {
		t.Fatalf("warp-blocked center still merged sides into component %d", c[0][0])
	}
	if c[0][1] != 0 {
		t.Fatalf("blocked pad itself got component %d, want 0", c[0][1])
	}
}
