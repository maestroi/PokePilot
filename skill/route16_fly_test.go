package skill

import "testing"

func TestRoute16FlyHouseTransactionDestination(t *testing.T) {
	got, ok := Place(route16FlyHousePlace)
	if !ok {
		t.Fatal("Route 16 Fly house transaction destination is not registered")
	}
	want := Destination{Map: route16FlyHouseMap, X: route16FlyHouseStagingX, Y: route16FlyHouseStagingY}
	if got != want {
		t.Fatalf("Route 16 Fly house destination = %+v, want %+v", got, want)
	}
}
