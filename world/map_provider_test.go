package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

type fakeMapProvider struct{}

func (fakeMapProvider) MapIDs() []uint8 { return []uint8{1, 2} }

func (fakeMapProvider) ParseMap(id uint8) (worldmodel.MapHeader, error) {
	switch id {
	case 1:
		return worldmodel.MapHeader{
			ID: 1, WidthBlocks: 1, HeightBlocks: 1,
			Warps: []worldmodel.Warp{{X: 0, Y: 0, DestWarpID: 0, DestMap: 2}},
		}, nil
	case 2:
		return worldmodel.MapHeader{
			ID: 2, WidthBlocks: 1, HeightBlocks: 1,
			Warps: []worldmodel.Warp{{X: 1, Y: 1, DestWarpID: 0, DestMap: 1}},
		}, nil
	default:
		return worldmodel.MapHeader{}, os.ErrNotExist
	}
}

func (fakeMapProvider) Grid(id uint8, _ []byte, _ worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	return worldmodel.GridSpec{
		MapID:         id,
		Width:         2,
		Height:        2,
		Walkable:      []bool{true, true, true, true},
		CollisionTile: []uint8{1, 1, 1, 1},
		FieldTile:     []uint8{1, 1, 1, 1},
	}, nil
}

func (fakeMapProvider) LookupElevator(uint8) (worldmodel.ElevatorSpec, bool) {
	return worldmodel.ElevatorSpec{}, false
}

func (fakeMapProvider) ElevatorFloorForDestination(uint8, uint8) (worldmodel.ElevatorFloor, bool) {
	return worldmodel.ElevatorFloor{}, false
}

func TestBuildGraphFromFakeProvider(t *testing.T) {
	g, err := BuildGraph(fakeMapProvider{})
	if err != nil {
		t.Fatalf("BuildGraph(fake provider): %v", err)
	}
	if !hasWarpEdge(g, 1, 2, 0, 0) {
		t.Fatalf("fake provider graph missing 1->2 warp: %+v", g.Edges[1])
	}
	if !hasWarpEdge(g, 2, 1, 1, 1) {
		t.Fatalf("fake provider graph missing 2->1 warp: %+v", g.Edges[2])
	}
}

func TestWorldProductionFilesDoNotImportRedROM(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "github.com/maestroi/pokepilot/red/rom") {
			t.Errorf("%s imports red/rom; world must depend only on portable map contracts", name)
		}
	}
}
