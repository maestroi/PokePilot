package agent

import (
	"os"
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/worldmodel"
)

// Every Kanto leader stand tile must be ROM floor next to the leader; these
// were hand-written and Janine's (1,11) was a wall.
func TestGSKantoGymStandTilesAreWalkableBesideLeaders(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	provider := gsprofile.New(gsprofile.GoldGameID).NativeMapProvider(rom)
	for _, spec := range gsKantoGymStages {
		id, err := gsOpeningMapID(spec.gym)
		if err != nil {
			t.Fatal(err)
		}
		grid, err := provider.Grid(id, nil, worldmodel.TraversalLand)
		if err != nil {
			t.Fatal(err)
		}
		dx, dy := int(spec.sx)-int(spec.x), int(spec.sy)-int(spec.y)
		if !grid.Walkable[int(spec.sy)*grid.Width+int(spec.sx)] || dx*dx+dy*dy != 1 {
			t.Errorf("%s: stand (%d,%d) is not ROM floor beside the leader at (%d,%d)",
				spec.leader, spec.sx, spec.sy, spec.x, spec.y)
		}
	}
}
