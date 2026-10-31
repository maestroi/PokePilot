package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

// The League rooms form a physical corridor, but opening a battle-locked
// door remains the responsibility of the retail script after the trainer
// event. The graph must never invent a direct Pokecenter -> Champion warp.
func TestGen2LeagueNativeRoomsFollowRetailOrder(t *testing.T) {
	p := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(p)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{
		"INDIGO_PLATEAU_POKECENTER_1F", "WILLS_ROOM", "KOGAS_ROOM",
		"BRUNOS_ROOM", "KARENS_ROOM", "LANCES_ROOM", "HALL_OF_FAME",
	}
	route, err := world.FindNativeRoute(graph, nativeID(t, names[0]), nativeID(t, names[len(names)-1]))
	if err != nil {
		t.Fatal(err)
	}
	if len(route) != len(names)-1 {
		t.Fatalf("League route has %d legs, want %d: %+v", len(route), len(names)-1, route)
	}
	for i, step := range route {
		if step.From != nativeID(t, names[i]) || step.To != nativeID(t, names[i+1]) {
			t.Fatalf("League route leg %d = %+v; expected %s -> %s", i, step, names[i], names[i+1])
		}
	}
	for _, name := range names {
		header, err := p.ParseMap(nativeID(t, name))
		if err != nil {
			t.Fatal(err)
		}
		blocks := make([]byte, int(header.WidthBlocks)*int(header.HeightBlocks))
		if _, err := p.Grid(header.ID, blocks, 0); err != nil {
			t.Fatalf("%s has no verified retail collision tileset: %v", name, err)
		}
	}
}

// The Pokecenter exit to Will uses retail warp #1; Will's northern exit
// uses Koga warp #1. ParseMap converts these 1-based decomp IDs to 0-based
// route targets, so this pins both the coordinates and indexing.
func TestGen2LeagueWillWarpIDsAndCoordinates(t *testing.T) {
	p := NewFirstBadgeWorldProvider(nil)
	cases := []struct {
		from   string
		x, y   uint8
		to     string
		warpID uint8
	}{
		{"INDIGO_PLATEAU_POKECENTER_1F", 14, 3, "WILLS_ROOM", 0},
		{"WILLS_ROOM", 5, 17, "INDIGO_PLATEAU_POKECENTER_1F", 3},
		{"WILLS_ROOM", 4, 2, "KOGAS_ROOM", 0},
		{"WILLS_ROOM", 5, 2, "KOGAS_ROOM", 1},
	}
	for _, c := range cases {
		header, err := p.ParseMap(nativeID(t, c.from))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, warp := range header.Warps {
			if warp.X == c.x && warp.Y == c.y && warp.DestMap == nativeID(t, c.to) &&
				warp.DestWarpID == c.warpID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s (%d,%d) -> %s warp %d is missing or misindexed",
				c.from, c.x, c.y, c.to, c.warpID)
		}
	}
}
