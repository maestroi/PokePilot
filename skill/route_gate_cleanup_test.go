package skill

import "testing"

func TestRouteGateCleanupChoiceIndexDeclinesMuseumAdmission(t *testing.T) {
	index, ok := routeGateCleanupChoiceIndex(museum1FMap, "Would you like to come in?")
	if !ok {
		t.Fatal("Museum admission was not recognized as a reversible route-gate cleanup")
	}
	if index != 1 {
		t.Fatalf("Museum cleanup choice = %d, want NO index 1", index)
	}
}

func TestRouteGateCleanupChoiceIndexRejectsUnknownChoice(t *testing.T) {
	if index, ok := routeGateCleanupChoiceIndex(museum1FMap, "Would you like to buy MAGIKARP?"); ok {
		t.Fatalf("unknown gameplay choice was classified as cleanup: index=%d", index)
	}
	if index, ok := routeGateCleanupChoiceIndex(0x44, "Would you like to come in?"); ok {
		t.Fatalf("Museum text on another map was classified as cleanup: index=%d", index)
	}
}
