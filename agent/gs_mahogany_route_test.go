package agent

import "testing"

// Route interruptions may use the shared Gen-II wild/trainer battler and
// recovery using real event-bound Rocket B3F scripts.
func TestGSMahoganyRouteOwnershipStopsAtScriptedStoryBoundaries(t *testing.T) {
	mart, err := gsOpeningMapID("MAHOGANY_MART_1F")
	if err != nil || !gsSecondBadgeOwnedMap(mart) {
		t.Fatalf("Mahogany Mart Lance reveal must be owned: %#04x %v", mart, err)
	}
	for _, name := range []string{"TEAM_ROCKET_BASE_B1F", "TEAM_ROCKET_BASE_B2F"} {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("owned Rocket cameras / Lance heal missing map %s: %#04x %v", name, id, err)
		}
	}
	for _, name := range []string{
		"ROUTE_42_ECRUTEAK_GATE", "ROUTE_42", "MAHOGANY_TOWN",
		"MAHOGANY_POKECENTER_1F", "ROUTE_43_MAHOGANY_GATE",
		"ROUTE_43", "LAKE_OF_RAGE",
	} {
		id, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatal(err)
		}
		if !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("%s has no owned ordinary-interruption route", name)
		}
	}
	id, err := gsOpeningMapID("TEAM_ROCKET_BASE_B3F")
	if err != nil || !gsSecondBadgeOwnedMap(id) {
		t.Fatalf("Rocket B3F must be owned by password/story controller: %#04x %v", id, err)
	}
}
