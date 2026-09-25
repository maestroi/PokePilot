package skill

import "testing"

// Regression for #963: the old Route 25 destination was (44,3), the wall
// immediately west of Bill's House door warp at (45,3). GoTo correctly
// returned no_path when asked to stand on it. Keep the generic Route 25 place
// on the ordinary approach floor one tile south of the door instead.
func TestRoute25PlaceUsesBillsHouseApproachFloor(t *testing.T) {
	d, ok := Place("route 25")
	if !ok {
		t.Fatal("route 25 place missing")
	}
	want := (Destination{Map: 0x24, X: 45, Y: 4, Kind: DestinationMap})
	if d != want {
		t.Fatalf("route 25 destination = %+v, want %+v", d, want)
	}
}
