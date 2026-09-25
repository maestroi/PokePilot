package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestNamedGeographicPlacesUseMapArrivalSemantics(t *testing.T) {
	for _, name := range []string{"pallet town", "viridian city", "route 1", "viridian forest"} {
		dest, ok := Place(name)
		if !ok {
			t.Fatalf("Place(%q) missing", name)
		}
		if dest.Kind != DestinationMap {
			t.Fatalf("Place(%q) kind=%v, want DestinationMap", name, dest.Kind)
		}
	}
	center, ok := Place("viridian pokemon center")
	if !ok {
		t.Fatal("viridian pokemon center missing")
	}
	if center.Kind != DestinationExactTile {
		t.Fatalf("service place kind=%v, want exact legacy approach", center.Kind)
	}
}

func TestMapDestinationReachedAnywhereOnTargetMap(t *testing.T) {
	dest := Destination{Map: 0x03, X: 5, Y: 18, Kind: DestinationMap}
	if !dest.Reached(0x03, 39, 17) {
		t.Fatal("map destination should accept a non-canonical tile on the target map")
	}
	if dest.Reached(0x04, 5, 18) {
		t.Fatal("map destination accepted a different map")
	}
}

func TestAreaDestinationAcceptsAnyTileInRegion(t *testing.T) {
	dest := AreaDestination(0x0c, 4, 10, 8, 14)
	if !dest.Reached(0x0c, 7, 12) {
		t.Fatal("area destination rejected an interior tile")
	}
	if dest.Reached(0x0c, 9, 12) {
		t.Fatal("area destination accepted a tile outside the area")
	}
	if got := len(destinationRouteTargets(dest)); got != 25 {
		t.Fatalf("area route target count=%d, want 25", got)
	}
}

func TestInteractionDestinationRoutesToApproachTilesNotObjectTile(t *testing.T) {
	dest := InteractionDestination(0x29, 3, 1)
	targets := destinationRouteTargets(dest)
	if len(targets) != 7 {
		t.Fatalf("interaction approach targets=%d, want 7 valid in-bounds approaches", len(targets))
	}
	for _, target := range targets {
		if target.X == 3 && target.Y == 1 {
			t.Fatal("interaction route targets included the occupied object tile")
		}
	}
	wantCounterApproach := false
	for _, target := range targets {
		if target.X == 3 && target.Y == 3 {
			wantCounterApproach = true
		}
	}
	if !wantCounterApproach {
		t.Fatal("interaction targets omitted the two-tile Pokemon Center counter approach")
	}
}

func TestSemanticMapCostDoesNotChargeCanonicalTileDetour(t *testing.T) {
	g := &world.Graph{Edges: map[uint8][]world.Edge{1: nil}}
	from := Destination{Map: 1, X: 2, Y: 2}

	mapGoal := Destination{Map: 1, X: 50, Y: 50, Kind: DestinationMap}
	mapResult, err := routePlanToDestinationByTravelPolicy(nil, g, 1, 2, 2, mapGoal, nil, world.RoutePrerequisites{})
	if err != nil {
		t.Fatalf("map goal route cost: %v", err)
	}
	if mapResult.Cost != 0 {
		t.Fatalf("map goal cost=%d, want 0 after map arrival", mapResult.Cost)
	}

	exactGoal := Destination{Map: 1, X: 50, Y: 50}
	exactResult, err := routePlanToDestinationByTravelPolicy(nil, g, from.Map, int(from.X), int(from.Y), exactGoal, nil, world.RoutePrerequisites{})
	if err != nil {
		t.Fatalf("exact goal route cost: %v", err)
	}
	if exactResult.Cost <= mapResult.Cost {
		t.Fatalf("exact goal cost=%d, want greater than semantic map cost=%d", exactResult.Cost, mapResult.Cost)
	}
}
