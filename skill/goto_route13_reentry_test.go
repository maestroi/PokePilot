package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

func TestRoute13StationaryTrainerMakesRow8FreshReentry(t *testing.T) {
	p := os.Getenv("POKEMON_RED_ROM")
	if p == "" {
		t.Skip("POKEMON_RED_ROM required")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	g, err := world.BuildGraph(data)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	const (
		route13 = uint8(0x18)
		route14 = uint8(0x19)
	)
	h13, err := rom.ParseMap(data, route13)
	if err != nil {
		t.Fatalf("ParseMap Route 13: %v", err)
	}
	grid13, err := world.Build(data, h13)
	if err != nil {
		t.Fatalf("Build Route 13 grid: %v", err)
	}
	trainerSlot := 0
	for i, o := range h13.Objects {
		if o.X == 12 && o.Y == 4 {
			trainerSlot = i + 1
			break
		}
	}
	if trainerSlot == 0 {
		t.Fatal("Route 13 object at (12,4) is missing")
	}
	observed13 := observedStationaryObjectBlockers(h13, []state.SpriteState{{Slot: trainerSlot, X: 12, Y: 4}})
	if !observed13[[2]int{12, 4}] {
		t.Fatal("Route 13 object at (12,4) is no longer a visible MovementStay blocker")
	}
	g, err = overlayObservedMapTopology(g, grid13, h13, observed13)
	if err != nil {
		t.Fatalf("overlay Route 13: %v", err)
	}

	// GoTo observes Route 14 after leaving Route 13. Updating Route 14 must
	// retain Route 13's stationary-trainer split for the return leg.
	h14, err := rom.ParseMap(data, route14)
	if err != nil {
		t.Fatalf("ParseMap Route 14: %v", err)
	}
	grid14, err := world.Build(data, h14)
	if err != nil {
		t.Fatalf("Build Route 14 grid: %v", err)
	}
	g, err = overlayObservedMapTopology(g, grid14, h14, nil)
	if err != nil {
		t.Fatalf("overlay Route 14: %v", err)
	}

	var row4, row8 *world.Edge
	for i := range g.Edges[route14] {
		e := &g.Edges[route14][i]
		if e.To != route13 || e.Kind != world.EdgeConnection {
			continue
		}
		start, end, ok := world.ConnectionBand(*e)
		if !ok {
			continue
		}
		if start <= 4 && 4 <= end {
			row4 = e
		}
		if start <= 8 && 8 <= end {
			row8 = e
		}
	}
	if row4 == nil || row8 == nil {
		t.Fatalf("missing Route 14 -> Route 13 bands: row4=%v row8=%v", row4, row8)
	}

	if same, known := g.EdgeEntrySharesComponentWith(*row4, 11, 4); !known || !same {
		t.Fatalf("row-4 entry relative to (11,4) = same %t, known %t; want same", same, known)
	}
	if same, known := g.EdgeEntrySharesComponentWith(*row8, 11, 4); !known || same {
		t.Fatalf("row-8 entry relative to (11,4) = same %t, known %t; want fresh component", same, known)
	}
}
