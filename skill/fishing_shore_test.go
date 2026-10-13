package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// TestFishingShoreWarpComponentReachable pins the invariant Fish relies on when
// a map's only fishable water sits in a walkable component that land BFS cannot
// reach from the usual overworld arrival tile. Fuchsia City's Super Rod pond is
// north of the Safari Zone office: arriving on the city street reports
// fishing_no_shoreline unless shoreline selection falls back to the same
// component-aware warp graph Travel uses to enter that courtyard.
func TestFishingShoreWarpComponentReachable(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read POKEMON_RED_ROM: %v", err)
	}

	const fuchsiaCity uint8 = 0x07
	provider := rom.NewWorldProvider(romData)
	landSpec, err := provider.Grid(fuchsiaCity, nil, worldmodel.TraversalLand)
	if err != nil {
		t.Fatalf("land grid: %v", err)
	}
	waterSpec, err := provider.Grid(fuchsiaCity, nil, worldmodel.TraversalWater)
	if err != nil {
		t.Fatalf("water grid: %v", err)
	}
	land, err := world.GridFromSpec(landSpec)
	if err != nil {
		t.Fatalf("land FromSpec: %v", err)
	}
	water, err := world.GridFromSpec(waterSpec)
	if err != nil {
		t.Fatalf("water FromSpec: %v", err)
	}

	// East seam arrival — where Travel lands from Route 15 for this farm case.
	const sx, sy = 39, 16
	candidates := fishingShoreCandidates(land, water)
	if len(candidates) == 0 {
		t.Fatal("Fuchsia City has no fishing shore candidates in ROM geometry")
	}
	if _, ok := pickFishingShore(candidates, land, sx, sy, nil, nil); ok {
		t.Fatal("expected no same-component land path from (39,16) to a Fuchsia fishing shore")
	}

	g, err := routeGraphForROM(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	shore, ok := pickFishingShore(candidates, land, sx, sy, nil, fishingShoreRouteCosts(g, fuchsiaCity, sx, sy, candidates))
	if !ok {
		t.Fatal("expected a warp-mediated fishing shore from Fuchsia (39,16)")
	}
	// Courtyard pond south bank (measured): stand tiles face water at y=21.
	if shore.standY != 22 || shore.waterY != 21 || shore.standX < 29 || shore.standX > 33 {
		t.Fatalf("warp shore = %+v, want courtyard stand y=22 x in [29,33] facing water y=21", shore)
	}
	if shore.distance != 2 {
		t.Fatalf("warp shore distance = %d, want 2 (office out-and-back)", shore.distance)
	}
}
