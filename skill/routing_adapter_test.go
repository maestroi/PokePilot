package skill

import (
	"os"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

type fakeRoutingMemory struct{}

func (fakeRoutingMemory) Peek8(uint16) byte       { return 0 }
func (fakeRoutingMemory) PeekInto(uint16, []byte) {}

type fakeRoutingDecoder struct {
	state game.LiveTopologyState
}

type fakeRoutingOverworldDecoder struct {
	state game.OverworldState
}

func (f fakeRoutingOverworldDecoder) DecodeOverworld(game.MemoryReader) game.OverworldState {
	return f.state
}

type fakeGen2ElevatorTransitionDecoder struct {
	ready bool
	seen  game.ElevatorTransition
}

func (f *fakeGen2ElevatorTransitionDecoder) ElevatorTransitionReady(_ game.MemoryReader, transition game.ElevatorTransition) bool {
	f.seen = transition
	return f.ready
}

func (f fakeRoutingDecoder) DecodeLiveTopology(game.MemoryReader) (game.LiveTopologyState, error) {
	return f.state, nil
}

type fakeGen2MapProvider struct{}

func (fakeGen2MapProvider) MapIDs() []uint8 { return []uint8{0x42} }

func (fakeGen2MapProvider) ParseMap(id uint8) (worldmodel.MapHeader, error) {
	return worldmodel.MapHeader{
		ID:           id,
		WidthBlocks:  1,
		HeightBlocks: 1,
		Objects: []worldmodel.MapObject{
			{Slot: 1, X: 1, Y: 0, Movement: worldmodel.ObjectMovementStay, Role: worldmodel.InteractionPokemonCenterNurse},
		},
	}, nil
}

func (fakeGen2MapProvider) Grid(id uint8, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	walkable := []bool{true, false, true, true}
	if mode == worldmodel.TraversalWater {
		walkable[1] = true
	}
	return worldmodel.GridSpec{
		MapID:         id,
		Width:         2,
		Height:        2,
		Walkable:      walkable,
		CollisionTile: []uint8{1, 2, 3, 4},
		FieldTile:     []uint8{1, 2, 3, 4},
		Cuttable:      []bool{false, false, true, false},
	}, nil
}

func (fakeGen2MapProvider) LookupElevator(uint8) (worldmodel.ElevatorSpec, bool) {
	return worldmodel.ElevatorSpec{}, false
}

func (fakeGen2MapProvider) ElevatorFloorForDestination(uint8, uint8) (worldmodel.ElevatorFloor, bool) {
	return worldmodel.ElevatorFloor{}, false
}

func TestLiveMapGridWithFakeGen2RoutingProfile(t *testing.T) {
	header := worldmodel.MapHeader{ID: 0x42, WidthBlocks: 1, HeightBlocks: 1}
	decoder := fakeRoutingDecoder{state: game.LiveTopologyState{
		NativeMapID:  0x42,
		WidthBlocks:  1,
		HeightBlocks: 1,
		Blocks:       []byte{0x99},
		Traversal:    game.TraversalLand,
	}}
	grid, err := liveMapGridWithRuntime(fakeRoutingMemory{}, decoder, fakeGen2MapProvider{}, header, worldmodel.TraversalLand)
	if err != nil {
		t.Fatalf("liveMapGridWithRuntime(fake Gen II): %v", err)
	}
	if grid.Width != 2 || grid.Height != 2 {
		t.Fatalf("grid = %dx%d, want 2x2", grid.Width, grid.Height)
	}
	if grid.Walkable(1, 0) {
		t.Fatal("land grid unexpectedly made water cell walkable")
	}
	if !grid.Cuttable(0, 1) {
		t.Fatal("portable cuttable cell was not retained")
	}

	water, err := liveMapGridWithRuntime(fakeRoutingMemory{}, decoder, fakeGen2MapProvider{}, header, worldmodel.TraversalWater)
	if err != nil {
		t.Fatalf("water liveMapGridWithRuntime(fake Gen II): %v", err)
	}
	if !water.Walkable(1, 0) {
		t.Fatal("water traversal semantics did not come from fake provider")
	}
}

func TestPortableRoutingFilesDoNotImportRedROM(t *testing.T) {
	for _, path := range []string{
		"routing_runtime.go",
		"live_topology.go",
		"routable.go",
		"warp.go",
		"elevator.go",
		"intra_map_warp.go",
		"blockers.go",
		"connection_field_path.go",
		"field_path_bridge.go",
		"component_restage.go",
		"destination_navigation.go",
		"heal.go",
		"move.go",
		"goto.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "github.com/maestroi/pokepilot/red/rom") {
			t.Fatalf("%s imports red/rom; reusable routing must use the profile/worldmodel boundary", path)
		}
	}
}

func TestFakeGen2TransitionRuntimeUsesSemanticCurrentMap(t *testing.T) {
	for _, edge := range []world.Edge{
		{Kind: world.EdgeConnection, From: 0x42, To: 0x43},
		{Kind: world.EdgeWarp, From: 0x43, To: 0x44, WarpX: 6, WarpY: 7},
	} {
		decoder := fakeRoutingOverworldDecoder{state: game.OverworldState{
			NativeMapID: uint16(edge.From),
			X:           11,
			Y:           12,
		}}
		before, err := routingRuntimeStateWithDecoder(fakeRoutingMemory{}, decoder)
		if err != nil {
			t.Fatalf("decode source for edge %#v: %v", edge, err)
		}
		if before.Map != edge.From || before.X != 11 || before.Y != 12 {
			t.Fatalf("source runtime = %+v, want map %02x at (11,12)", before, edge.From)
		}

		decoder.state.NativeMapID = uint16(edge.To)
		after, err := routingRuntimeStateWithDecoder(fakeRoutingMemory{}, decoder)
		if err != nil {
			t.Fatalf("decode destination for edge %#v: %v", edge, err)
		}
		if after.Map != edge.To {
			t.Fatalf("destination runtime map = %02x, want %02x for edge %#v", after.Map, edge.To, edge)
		}
	}
}

func TestFakeGen2LiveTopologyDrivesObjectBlockers(t *testing.T) {
	header := worldmodel.MapHeader{
		ID: 0x42,
		Objects: []worldmodel.MapObject{
			{Slot: 1, X: 2, Y: 3, Movement: worldmodel.ObjectMovementStay},
			{Slot: 2, X: 4, Y: 5, Movement: worldmodel.ObjectMovementStay},
			{Slot: 3, X: 6, Y: 7, Movement: worldmodel.ObjectMovementWalk},
		},
	}
	live := game.LiveTopologyState{
		NativeMapID: 0x42,
		LiveObjects: []game.LiveMapObject{
			{Slot: 1, X: 9, Y: 3},
			{Slot: 3, X: 7, Y: 7},
		},
		ObjectPositions: map[int]game.MapPoint{
			1: {X: 9, Y: 3},
			2: {X: 4, Y: 5},
		},
		HiddenObjects: map[int]bool{2: true},
	}

	sprites := spriteBlockersFromTopology(live)
	if !sprites[[2]int{9, 3}] || !sprites[[2]int{7, 7}] {
		t.Fatalf("live sprite blockers = %v, want moved stay and moving object", sprites)
	}

	stable := stationaryObjectBlockers(header, objectTileSet(live), hiddenObjectSet(live))
	if !stable[[2]int{9, 3}] {
		t.Fatalf("moved stationary object did not follow semantic object position: %v", stable)
	}
	if stable[[2]int{2, 3}] {
		t.Fatalf("moved stationary object still blocks header home: %v", stable)
	}
	if stable[[2]int{4, 5}] {
		t.Fatalf("hidden stationary object still blocks: %v", stable)
	}
}

func TestFakeGen2ElevatorTransitionCapabilityReceivesSemanticRequest(t *testing.T) {
	header := worldmodel.MapHeader{
		ID: 0x50,
		Warps: []worldmodel.Warp{
			{X: 1, Y: 2},
			{X: 1, Y: 3},
		},
	}
	floor := worldmodel.ElevatorFloor{MapID: 0x62, DestWarpID: 3}
	request := elevatorTransitionRequest(header, floor)
	fake := &fakeGen2ElevatorTransitionDecoder{ready: true}
	if !fake.ElevatorTransitionReady(fakeRoutingMemory{}, request) {
		t.Fatal("fake transition capability unexpectedly reported not ready")
	}
	if fake.seen.SourceMapID != 0x50 || fake.seen.DestinationMapID != 0x62 || fake.seen.DestinationWarp != 3 {
		t.Fatalf("semantic elevator request = %+v", fake.seen)
	}
	if len(fake.seen.Doors) != 2 || fake.seen.Doors[0] != (game.MapPoint{X: 1, Y: 2}) || fake.seen.Doors[1] != (game.MapPoint{X: 1, Y: 3}) {
		t.Fatalf("semantic elevator doors = %+v", fake.seen.Doors)
	}
}

func TestReusableRoutingExecutionFilesHaveNoConcreteRedImports(t *testing.T) {
	for _, path := range []string{
		"warp.go",
		"elevator.go",
		"connection_field_path.go",
		"intra_map_warp.go",
		"blockers.go",
		"routable.go",
		"field_path_bridge.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"github.com/maestroi/pokepilot/red/state",
			"github.com/maestroi/pokepilot/red/sym",
			"github.com/maestroi/pokepilot/red/rom",
			"github.com/maestroi/pokepilot/red/combat",
		} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s imports concrete Red dependency %q", path, forbidden)
			}
		}
	}
}
