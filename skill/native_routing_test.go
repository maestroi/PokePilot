package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"

	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

func nativeTestGrid(t *testing.T, width, height int, walkable []bool) *world.NativeGrid {
	t.Helper()
	spec := worldmodel.NativeGridSpec{
		MapID:         0x1804,
		Width:         width,
		Height:        height,
		Walkable:      walkable,
		CollisionTile: make([]uint8, width*height),
	}
	grid, err := world.NativeGridFromSpec(spec)
	if err != nil {
		t.Fatalf("NativeGridFromSpec: %v", err)
	}
	return grid
}

func TestNativeAdjacentApproachStopsBesideWarp(t *testing.T) {
	grid := nativeTestGrid(t, 5, 5, []bool{
		true, true, true, true, true,
		true, true, true, true, true,
		true, true, true, true, true,
		true, true, true, true, true,
		true, true, true, true, true,
	})
	blocked := map[[2]int]bool{{2, 2}: true}
	path, push, err := nativeAdjacentApproach(grid, 0, 2, 2, 2, blocked)
	if err != nil {
		t.Fatalf("nativeAdjacentApproach: %v", err)
	}
	if len(path) != 1 || path[0] != (world.NativeStep{DX: 1}) {
		t.Fatalf("path = %+v, want one step right to (1,2)", path)
	}
	if push != (world.NativeStep{DX: 1}) {
		t.Fatalf("push = %+v, want right", push)
	}
}

type nativeConnectionTestProvider map[uint16]worldmodel.NativeMapHeader

func (p nativeConnectionTestProvider) MapIDs() []uint16 {
	out := make([]uint16, 0, len(p))
	for id := range p {
		out = append(out, id)
	}
	return out
}

func (p nativeConnectionTestProvider) ParseMap(id uint16) (worldmodel.NativeMapHeader, error) {
	return p[id], nil
}

func TestNativeConnectionApproachHonorsOffsetBounds(t *testing.T) {
	grid := nativeTestGrid(t, 6, 4, []bool{
		true, true, true, true, true, true,
		true, true, true, true, true, true,
		true, true, true, true, true, true,
		true, true, true, true, true, true,
	})
	provider := nativeConnectionTestProvider{
		0x1804: {ID: 0x1804, WidthBlocks: 3, HeightBlocks: 2},
		0x1803: {ID: 0x1803, WidthBlocks: 2, HeightBlocks: 2},
	}
	edge := world.NativeEdge{
		Kind:   world.EdgeConnection,
		From:   0x1804,
		To:     0x1803,
		Dir:    0,
		Offset: 2,
	}
	live := game.LiveTopologyState{NativeMapID: 0x1804, WidthBlocks: 3, HeightBlocks: 2}
	path, push, err := nativeConnectionApproach(provider, grid, live, edge, 5, 3, nil)
	if err != nil {
		t.Fatalf("nativeConnectionApproach: %v", err)
	}
	if push != (world.NativeStep{DY: -1}) {
		t.Fatalf("push = %+v, want up", push)
	}
	// Destination width is four tiles. Offset 2 is in blocks, so destination
	// x = source x - 4 and only source x=4..7 (of 0..5) are valid; from (5,3),
	// x=5 is the nearest valid north-edge crossing.
	if len(path) != 3 {
		t.Fatalf("path length = %d, want 3 to north edge at x=5: %+v", len(path), path)
	}
}

func TestNativeRuntimeBlockersAvoidWarpsAndObjects(t *testing.T) {
	live := game.LiveTopologyState{
		LiveObjects: []game.LiveMapObject{{Slot: 1, X: 3, Y: 4}},
	}
	header := worldmodel.NativeMapHeader{
		Warps: []worldmodel.NativeWarp{
			{X: 1, Y: 2},
			{X: 5, Y: 6},
		},
	}
	allow := [2]int{1, 2}
	blocked := nativeRuntimeBlockers(live, header, &allow)
	if blocked[[2]int{1, 2}] {
		t.Fatal("allowed warp was blocked")
	}
	if !blocked[[2]int{5, 6}] {
		t.Fatal("other warp was not blocked")
	}
	if !blocked[[2]int{3, 4}] {
		t.Fatal("live object was not blocked")
	}
	static := nativeStaticBlockers(header, &allow)
	if static[[2]int{3, 4}] {
		t.Fatal("static blockers must omit live sprites")
	}
	if !static[[2]int{5, 6}] {
		t.Fatal("static blockers must still include warps")
	}
}

// TestNativeErrNoPathCausedBySprites pins the Sprout Tower entrance contract:
// a corridor that opens when sprites are ignored is a transient block, not a
// sealed (map, entry, edge) proof. Marking those edges permanently unreachable
// is what looped run-x330xhsmcfod at SPROUT_TOWER_1F (9,15).
func TestNativeErrNoPathCausedBySprites(t *testing.T) {
	if !nativeErrNoPathCausedBySprites(world.ErrNoPath, nil) {
		t.Fatal("sprite-only seal must be treated as transient")
	}
	if nativeErrNoPathCausedBySprites(world.ErrNoPath, world.ErrNoPath) {
		t.Fatal("geometry that stays sealed without sprites is permanent")
	}
	if nativeErrNoPathCausedBySprites(nil, nil) {
		t.Fatal("a successful approach is not a sprite-caused miss")
	}
	if nativeErrNoPathCausedBySprites(errors.New("other"), nil) {
		t.Fatal("non-ErrNoPath failures are not sprite-caused misses")
	}

	// Concrete floor: NPC on the only approach tile to a warp. With the sprite
	// the adjacent approach fails; without it the south tile is free.
	grid := nativeTestGrid(t, 5, 5, []bool{
		true, true, true, true, true,
		true, true, true, true, true,
		true, true, true, true, true,
		true, true, true, true, true,
		true, true, true, true, true,
	})
	header := worldmodel.NativeMapHeader{Warps: []worldmodel.NativeWarp{{X: 2, Y: 2}}}
	live := game.LiveTopologyState{LiveObjects: []game.LiveMapObject{
		{Slot: 1, X: 2, Y: 3},
		{Slot: 2, X: 1, Y: 2},
		{Slot: 3, X: 3, Y: 2},
		{Slot: 4, X: 2, Y: 1},
	}}
	withSprites := nativeRuntimeBlockers(live, header, nil)
	delete(withSprites, [2]int{0, 2})
	_, _, errSprites := nativeAdjacentApproach(grid, 0, 2, 2, 2, withSprites)
	withoutSprites := nativeStaticBlockers(header, nil)
	delete(withoutSprites, [2]int{0, 2})
	_, _, errStatic := nativeAdjacentApproach(grid, 0, 2, 2, 2, withoutSprites)
	if !nativeErrNoPathCausedBySprites(errSprites, errStatic) {
		t.Fatalf("surrounded warp approach: with=%v without=%v; want sprite-caused ErrNoPath", errSprites, errStatic)
	}
}

// TestNativeAvoidUnderfootLeave pins the Sprout Tower 2F contract: after the
// center landing proves 3F unreachable, the graph ties between reversing
// through those stairs and leaving via a side stair. Standing on the chosen
// warp must prefer the sibling route (farm run-x330xhsmcfod).
func TestNativeAvoidUnderfootLeave(t *testing.T) {
	underfoot := world.NativeEdge{Kind: world.EdgeWarp, From: 0x0302, To: 0x0301, WarpX: 6, WarpY: 4, DestWarp: 2}
	sibling := world.NativeEdge{Kind: world.EdgeWarp, From: 0x0302, To: 0x0301, WarpX: 17, WarpY: 3, DestWarp: 4}
	route := []world.NativeEdge{underfoot, {Kind: world.EdgeWarp, From: 0x0301, To: 0x0302, WarpX: 2, WarpY: 6}}
	got := nativeAvoidUnderfootLeave(route, 6, 4, func(avoid world.NativeEdge) ([]world.NativeEdge, error) {
		if avoid != underfoot {
			t.Fatalf("avoid = %+v, want underfoot edge", avoid)
		}
		return []world.NativeEdge{sibling, route[1]}, nil
	})
	if len(got) == 0 || got[0] != sibling {
		t.Fatalf("route = %+v, want sibling leave %+v", got, sibling)
	}
	kept := nativeAvoidUnderfootLeave(route, 5, 4, func(world.NativeEdge) ([]world.NativeEdge, error) {
		t.Fatal("alternate must not run when not standing on the leave warp")
		return nil, nil
	})
	if len(kept) == 0 || kept[0] != underfoot {
		t.Fatalf("off-warp route = %+v, want original", kept)
	}
}

// nativeShellTestProvider is a one-map provider that can build a spec, so the
// settled-map gate can be exercised without a cartridge: the question is only
// whether routing is willing to read a block buffer the profile does not vouch
// for.
type nativeShellTestProvider struct {
	header worldmodel.NativeMapHeader
}

func (p nativeShellTestProvider) MapIDs() []uint16 { return []uint16{p.header.ID} }

func (p nativeShellTestProvider) ParseMap(id uint16) (worldmodel.NativeMapHeader, error) {
	return p.header, nil
}

func (p nativeShellTestProvider) Grid(mapID uint16, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.NativeGridSpec, error) {
	width, height := int(p.header.WidthBlocks), int(p.header.HeightBlocks)
	spec := worldmodel.NativeGridSpec{
		MapID:         mapID,
		Width:         width * 2,
		Height:        height * 2,
		Walkable:      make([]bool, width*height*4),
		CollisionTile: make([]uint8, width*height*4),
		Traversal:     mode,
	}
	for i := range spec.Walkable {
		spec.Walkable[i] = true
	}
	return spec, nil
}

// nativeShellTestReader is an empty memory surface: the gate under test reads
// the decoder, not RAM.
type nativeShellTestReader struct{}

func (nativeShellTestReader) Peek8(uint16) byte { return 0 }

func (nativeShellTestReader) PeekInto(uint16, []byte) {}

type nativeShellTestDecoder struct {
	live game.LiveTopologyState
}

func (d nativeShellTestDecoder) DecodeLiveTopology(game.MemoryReader) (game.LiveTopologyState, error) {
	return d.live, nil
}

// A map whose identity is current while a field script still owns the
// overworld has a block buffer holding the previous map's bytes. Routing must
// report that as a not-ready map instead of decoding it as collision.
func TestNativeLiveGridRefusesUnsettledMapShell(t *testing.T) {
	provider := nativeShellTestProvider{header: worldmodel.NativeMapHeader{ID: 0x1803, WidthBlocks: 2, HeightBlocks: 2}}
	live := game.LiveTopologyState{
		NativeMapID:   0x1803,
		WidthBlocks:   2,
		HeightBlocks:  2,
		Blocks:        []byte{0xff, 0xff, 0xff, 0xff},
		MapShellPhase: game.MapShellSettled,
	}
	_, _, _, err := nativeLiveGrid(nativeShellTestReader{}, nativeShellTestDecoder{live: live}, provider, 0x1803)
	if err == nil {
		t.Fatal("nativeLiveGrid decoded an unsettled map shell")
	}
	if !errors.Is(err, errLiveMapNotSettled) {
		t.Fatalf("nativeLiveGrid err = %v, want %v", err, errLiveMapNotSettled)
	}

	live.BlocksSettled = true
	if _, _, _, err := nativeLiveGrid(nativeShellTestReader{}, nativeShellTestDecoder{live: live}, provider, 0x1803); err != nil {
		t.Fatalf("nativeLiveGrid on a settled map: %v", err)
	}
}

// TestNativeArrivalRequiresControlHandoff pins the arrival half of the native
// route postcondition. Reaching the destination tile while a player event still
// owns the overworld is not arrival: the step that got there can roll a wild
// encounter, cross a trainer sightline or start a forced map script, and the
// caller must settle that typed interruption before anything presses a button.
// Regression for farm run run-11dd5ya1qev0ry, where Ilex Forest (20,23) was
// reached mid-encounter and the following Face then polled a direction the
// encounter script would never apply.
func TestNativeArrivalRequiresControlHandoff(t *testing.T) {
	const mapIlex uint16 = 0x032c
	dest := ExactNativeDestination(mapIlex, 20, 23)

	tests := []struct {
		name  string
		state game.OverworldState
		want  nativeArrivalState
	}{
		{
			name:  "different tile",
			state: game.OverworldState{NativeMapID: mapIlex, X: 20, Y: 24, Controllable: true},
			want:  nativeArrivalPending,
		},
		{
			name:  "different map",
			state: game.OverworldState{NativeMapID: 0x032b, X: 20, Y: 23, Controllable: true},
			want:  nativeArrivalPending,
		},
		{
			name:  "destination with control handed back",
			state: game.OverworldState{NativeMapID: mapIlex, X: 20, Y: 23, Controllable: true, MovementIdle: true},
			want:  nativeArrivalComplete,
		},
		{
			// Measured on the replayed checkpoint: the encounter script is
			// running (SCRIPT_READ, SCRIPT_RUNNING) while BattleMode is not set
			// yet, so this is neither a dialogue nor a battle the caller can see.
			name:  "destination during a wild encounter transition",
			state: game.OverworldState{NativeMapID: mapIlex, X: 20, Y: 23, Controllable: false},
			want:  nativeArrivalInterrupted,
		},
		{
			name:  "destination owned by a scripted movement",
			state: game.OverworldState{NativeMapID: mapIlex, X: 20, Y: 23, Controllable: false, MovementIdle: false},
			want:  nativeArrivalInterrupted,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nativeArrival(tc.state, dest); got != tc.want {
				t.Fatalf("nativeArrival(%+v) = %d, want %d", tc.state, got, tc.want)
			}
		})
	}
}

func TestMarkNativeObjectObstaclesLeavesRockOccupied(t *testing.T) {
	grid := nativeTestGrid(t, 5, 1, []bool{true, true, true, true, true})
	live := game.LiveTopologyState{LiveObjects: []game.LiveMapObject{
		{Slot: 1, X: 2, Y: 0, Clearable: game.FieldMoveRockSmash},
		{Slot: 2, X: 3, Y: 0}, // an ordinary NPC is not an obstacle
	}}
	markNativeObjectObstacles(grid, live)
	blocked := nativeRuntimeBlockers(live, worldmodel.NativeMapHeader{}, nil)
	if grid.Obstacle(2, 0) != worldmodel.ObstacleSmashRock || grid.Obstacle(3, 0) != "" {
		t.Fatalf("obstacles = %q/%q, want rock_smash/none", grid.Obstacle(2, 0), grid.Obstacle(3, 0))
	}
	if _, err := world.FindNativePath(grid, 0, 0, 4, 0, blocked); !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("ordinary path through a rock = %v, want ErrNoPath", err)
	}
	// An NPC two tiles later keeps the route sealed even when the rock is usable.
	smash := func(k worldmodel.NativeObstacle) bool { return k == worldmodel.ObstacleSmashRock }
	if _, err := world.FindNativeObstacleApproach(grid, 0, 0, 4, 0, blocked, smash); !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("smashing the rock must not path through the NPC behind it: %v", err)
	}
}

func TestNativeObstacleFieldMoveCoversEveryKind(t *testing.T) {
	want := map[worldmodel.NativeObstacle]FieldMove{
		worldmodel.ObstacleCutTree:   FieldCut,
		worldmodel.ObstacleSmashRock: FieldRockSmash,
		worldmodel.ObstacleWhirlpool: FieldWhirlpool,
	}
	for kind, move := range want {
		got, ok := nativeObstacleFieldMove(kind)
		if !ok || got != move {
			t.Fatalf("%s -> %v,%v; want %v", kind, got, ok, move)
		}
	}
	if _, ok := nativeObstacleFieldMove("strength_boulder"); ok {
		t.Fatal("an unmodelled obstacle kind must not map to a field move")
	}
}
