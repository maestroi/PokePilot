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
