package world

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
	"github.com/maestroi/pokepilot/worldverify"
)

type fakeMapProvider struct{}

func (fakeMapProvider) MapIDs() []worldmodel.MapID { return []worldmodel.MapID{1, 2} }

func (fakeMapProvider) ParseMap(id worldmodel.MapID) (worldmodel.MapHeader, error) {
	switch id {
	case 1:
		return worldmodel.MapHeader{
			ID: 1, WidthBlocks: 1, HeightBlocks: 1,
			Warps: []worldmodel.Warp{
				{X: 0, Y: 0, DestWarpID: 0, DestMap: 2},
				{X: 1, Y: 0, DestWarpID: 0, DestMap: 2, Inert: true},
			},
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

func (fakeMapProvider) Grid(id worldmodel.MapID, _ []byte, _ worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	return worldmodel.GridSpec{
		MapID:         id,
		Width:         2,
		Height:        2,
		Walkable:      []bool{true, true, true, true},
		CollisionTile: []uint8{1, 1, 1, 1},
		FieldTile:     []uint8{1, 1, 1, 1},
	}, nil
}

func (fakeMapProvider) LookupElevator(worldmodel.MapID) (worldmodel.ElevatorSpec, bool) {
	return worldmodel.ElevatorSpec{}, false
}

func (fakeMapProvider) ElevatorFloorForDestination(worldmodel.MapID, worldmodel.MapID) (worldmodel.ElevatorFloor, bool) {
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
	if hasWarpEdge(g, 1, 2, 1, 0) {
		t.Fatalf("fake provider graph emitted inert 1->2 warp: %+v", g.Edges[1])
	}

	blocked := warpTileBlockers([]worldmodel.Warp{
		{X: 0, Y: 0},
		{X: 1, Y: 0, Inert: true},
	})
	if !blocked[[2]int{0, 0}] {
		t.Fatal("active warp was not blocked from component flooding")
	}
	if blocked[[2]int{1, 0}] {
		t.Fatal("inert warp was incorrectly blocked from component flooding")
	}
}

type parseFailureProvider struct {
	failures map[worldmodel.MapID]error
	expected map[worldmodel.MapID]string
}

func (p parseFailureProvider) MapIDs() []worldmodel.MapID { return []worldmodel.MapID{1, 2, 3} }

func (p parseFailureProvider) ParseMap(id worldmodel.MapID) (worldmodel.MapHeader, error) {
	if err := p.failures[id]; err != nil {
		return worldmodel.MapHeader{}, err
	}
	return worldmodel.MapHeader{ID: id, WidthBlocks: 1, HeightBlocks: 1}, nil
}

func (parseFailureProvider) Grid(id worldmodel.MapID, _ []byte, _ worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	return worldmodel.GridSpec{
		MapID:         id,
		Width:         2,
		Height:        2,
		Walkable:      []bool{true, true, true, true},
		CollisionTile: []uint8{1, 1, 1, 1},
		FieldTile:     []uint8{1, 1, 1, 1},
	}, nil
}

func (parseFailureProvider) LookupElevator(worldmodel.MapID) (worldmodel.ElevatorSpec, bool) {
	return worldmodel.ElevatorSpec{}, false
}

func (parseFailureProvider) ElevatorFloorForDestination(worldmodel.MapID, worldmodel.MapID) (worldmodel.ElevatorFloor, bool) {
	return worldmodel.ElevatorFloor{}, false
}

func (p parseFailureProvider) ExpectedMapParseFailure(id worldmodel.MapID, _ error) (string, bool) {
	reason, ok := p.expected[id]
	return reason, ok
}

func TestBuildGraphAggregatesUnexpectedMapParseFailures(t *testing.T) {
	badHeader := errors.New("truncated map header")
	badObjects := errors.New("invalid object table")
	provider := parseFailureProvider{
		failures: map[worldmodel.MapID]error{2: badHeader, 3: badObjects},
	}

	g, err := BuildGraph(provider)
	if err == nil {
		t.Fatal("BuildGraph unexpectedly accepted provider parse failures")
	}
	if g != nil {
		t.Fatalf("partial runtime graph returned on parse failure: %+v", g)
	}
	var buildErr *GraphBuildError
	if !errors.As(err, &buildErr) {
		t.Fatalf("error type=%T, want *GraphBuildError: %v", err, err)
	}
	if len(buildErr.ParseFailures) != 2 {
		t.Fatalf("parse failures=%d, want 2: %+v", len(buildErr.ParseFailures), buildErr.ParseFailures)
	}
	if !errors.Is(err, badHeader) || !errors.Is(err, badObjects) {
		t.Fatalf("aggregated error does not unwrap underlying failures: %v", err)
	}
	message := err.Error()
	for _, want := range []string{"0x02", "truncated map header", "0x03", "invalid object table", "2 unexpected"} {
		if !strings.Contains(message, want) {
			t.Fatalf("BuildGraph error %q missing %q", message, want)
		}
	}
}

func TestBuildGraphKeepsExpectedParseFailureVisibleToWorldVerify(t *testing.T) {
	provider := parseFailureProvider{
		failures: map[worldmodel.MapID]error{2: errors.New("unused map layout is intentionally unsupported")},
		expected: map[worldmodel.MapID]string{2: "dead duplicate map excluded by this adapter"},
	}

	g, err := BuildGraph(provider)
	if err != nil {
		t.Fatalf("BuildGraph(expected parse failure): %v", err)
	}
	failures := g.ParseFailures()
	if len(failures) != 1 || failures[0].MapID != 2 || !failures[0].Expected {
		t.Fatalf("retained parse failures=%+v, want expected map 0x02", failures)
	}

	report := VerifyGraph(g, nil, 1)
	if report.HasErrors() {
		t.Fatalf("expected parse omission should be audit-visible, not fatal: %+v", report.Findings)
	}
	if report.Stats.ExpectedMapParseFailures != 1 {
		t.Fatalf("expected parse failures=%d, want 1", report.Stats.ExpectedMapParseFailures)
	}
	if !reportHasFinding(report, "expected_map_parse_failure", worldverify.SeverityWarning) {
		t.Fatalf("worldverify did not surface expected parse failure: %+v", report.Findings)
	}
}

type wideMapProvider struct{}

func (wideMapProvider) MapIDs() []worldmodel.MapID { return []worldmodel.MapID{0x0101} }

func (wideMapProvider) ParseMap(id worldmodel.MapID) (worldmodel.MapHeader, error) {
	return worldmodel.MapHeader{ID: id, WidthBlocks: 1, HeightBlocks: 1}, nil
}

func (wideMapProvider) Grid(id worldmodel.MapID, _ []byte, mode worldmodel.TraversalMode) (worldmodel.GridSpec, error) {
	return worldmodel.GridSpec{
		MapID:         id,
		Width:         2,
		Height:        2,
		Walkable:      []bool{true, true, true, true},
		CollisionTile: []uint8{1, 1, 1, 1},
		FieldTile:     []uint8{1, 1, 1, 1},
		Traversal:     mode,
	}, nil
}

func (wideMapProvider) LookupElevator(worldmodel.MapID) (worldmodel.ElevatorSpec, bool) {
	return worldmodel.ElevatorSpec{}, false
}

func (wideMapProvider) ElevatorFloorForDestination(worldmodel.MapID, worldmodel.MapID) (worldmodel.ElevatorFloor, bool) {
	return worldmodel.ElevatorFloor{}, false
}

func TestBuildGraphPreservesWideMapID(t *testing.T) {
	g, err := BuildGraph(wideMapProvider{})
	if err != nil {
		t.Fatalf("BuildGraph(wide provider): %v", err)
	}
	if _, ok := g.Edges[0x0101]; !ok {
		t.Fatalf("wide map id 0x0101 was truncated: keys=%v", g.Edges)
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
