package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// Route 22 Gate's south door and north exit are all "LAST_MAP, 1/2"; the
// gate script resolves LAST_MAP from wYCoord. A shared DestWarpID must not
// make the south door a Route 23 candidate: standing on it after entering,
// Traverse pushed down and walked straight back to Route 22
// (run-1biaubd9xooqm).
func TestRoute22GateNorthEdgeExcludesSouthDoor(t *testing.T) {
	romData := badgeFourROM(t)
	const gate, route23 = 0xc1, 0x22
	h, err := rom.ParseMap(romData, gate)
	if err != nil {
		t.Fatalf("ParseMap(route 22 gate): %v", err)
	}
	e := world.Edge{Kind: world.EdgeWarp, From: gate, To: route23, WarpX: 4, WarpY: 0}
	got := edgeWarpCandidates(h, e, romData)
	if len(got) == 0 {
		t.Fatal("no Route 23 candidates in Route 22 Gate")
	}
	for _, w := range got {
		if w.Y != 0 {
			t.Fatalf("Route 23 candidate (%d,%d) is not a north exit: %v", w.X, w.Y, got)
		}
	}
}
