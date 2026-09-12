package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// TestVermilionCityHasReachableCutTreeFromCityPlace is the ROM half of the
// farm failure "EnterVermilionGym: no reachable Cut tree found near gym
// warp (12,19)". The gym courtyard is a closed component until the overworld
// Cut tree is removed; Travel's shared candidate scan must see that tree
// from Vermilion City's Place tile. FieldTile-only matching misses it because
// this block exposes $3d on the collision subtile.
func TestVermilionCityHasReachableCutTreeFromCityPlace(t *testing.T) {
	romData := badgeFourROM(t)
	start, ok := Place("vermilion city")
	if !ok {
		t.Fatal("vermilion city place missing")
	}
	h, err := rom.ParseMap(romData, vermilionCity)
	if err != nil {
		t.Fatalf("ParseMap(vermilion): %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(vermilion): %v", err)
	}

	sx, sy := int(start.X), int(start.Y)
	var reached bool
	for _, c := range routeCutCandidates(grid, h.Tileset, sx, sy) {
		if _, ok := reachableBesideOnMap(grid, vermilionCity, sx, sy, c.x, c.y, nil); ok {
			reached = true
			break
		}
	}
	if !reached {
		t.Fatalf("no reachable Cut tree from vermilion city place (%d,%d); gym warp stays in a closed courtyard", sx, sy)
	}
}
