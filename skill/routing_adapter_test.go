package skill

import (
	"os"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/worldmodel"
)

type fakeRoutingMemory struct{}

func (fakeRoutingMemory) Peek8(uint16) byte { return 0 }
func (fakeRoutingMemory) PeekInto(uint16, []byte) {}

type fakeRoutingDecoder struct {
	state game.LiveTopologyState
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
		Traversal:    worldmodel.TraversalLand,
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
