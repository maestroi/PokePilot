package agent

import "testing"

// Route interruptions may use the shared Gen-II wild/trainer battler and
// recovery without granting ownership of the Rocket and Pryce scripts.
func TestGSMahoganyRouteOwnershipStopsAtScriptedStoryBoundaries(t *testing.T) {
	mart, err := gsOpeningMapID("MAHOGANY_MART_1F")
	if err != nil || !gsSecondBadgeOwnedMap(mart) {
		t.Fatalf("Mahogany Mart Lance reveal must be owned: %#04x %v", mart, err)
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
	for _, name := range []string{"MAHOGANY_GYM", "TEAM_ROCKET_BASE_B1F", "TEAM_ROCKET_BASE_B2F", "TEAM_ROCKET_BASE_B3F"} {
		id, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatal(err)
		}
		if gsSecondBadgeOwnedMap(id) {
			t.Fatalf("%s must stay unowned until its story controller exists", name)
		}
	}
}
