package data

import "testing"

func TestWildCaptureBallOrderIsStrongestFirstWithoutMasterBall(t *testing.T) {
	got := WildCaptureBallOrder()
	want := []uint8{staticUltraBall, staticGreatBall, staticPokeBall}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
