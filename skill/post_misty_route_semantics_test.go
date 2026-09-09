package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestBillProgressionAvailableAfterMistyAndBlackout(t *testing.T) {
	for _, mapID := range []uint8{0x41, 0x40, 0x03, 0x23, 0x24, billsHouseMap} {
		if !BillProgressionAvailable(mapID) {
			t.Errorf("Bill progression unavailable on map %#04x", mapID)
		}
	}
	if BillProgressionAvailable(0x00) {
		t.Fatal("Bill progression unexpectedly available in Pallet Town")
	}
}

func TestRedRouteCapabilitiesProjectSSTicketGate(t *testing.T) {
	mem := new(state.Mem)
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = ssTicketItem
	mem[sym.BagItems+1] = 1
	caps := redRouteCapabilities(nil, mem)
	if !caps.Has(capCanPassCeruleanRobbedHouse) {
		t.Fatalf("S.S. Ticket did not project robbed-house capability: %v", caps)
	}
}

func TestPostMistyRouteTransitionsExposeBillAndCutGates(t *testing.T) {
	billEdge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  semanticCeruleanCityMap,
		To:    ceruleanTrashedHouseMap,
		WarpX: ceruleanTrashedHouseFrontWarpX,
		WarpY: ceruleanTrashedHouseFrontWarpY,
	}
	bill, ok := redRouteTransitionForEdge(billEdge)
	if !ok {
		t.Fatal("Cerulean robbed-house front door has no semantic transition")
	}
	if !bill.Gate || len(bill.Requires) != 1 || bill.Requires[0] != capCanPassCeruleanRobbedHouse {
		t.Fatalf("Bill gate = %+v", bill)
	}

	// The rear hole is the component-changing exit after Bill; it must not
	// itself be misclassified as the guarded front door.
	rear := billEdge
	rear.WarpY = 9
	if transition, ok := redRouteTransitionForEdge(rear); ok && transition.ID == "red:cerulean_robbed_house" {
		t.Fatalf("Cerulean rear hole incorrectly classified as Bill gate: %+v", transition)
	}

	route9Edge := world.Edge{Kind: world.EdgeConnection, From: semanticCeruleanCityMap, To: semanticRoute9Map}
	route9, ok := redRouteTransitionForEdge(route9Edge)
	if !ok {
		t.Fatal("Cerulean -> Route 9 has no semantic Cut transition")
	}
	if !route9.Gate || len(route9.Requires) != 1 || route9.Requires[0] != capCanCut {
		t.Fatalf("Route 9 gate = %+v", route9)
	}
}
