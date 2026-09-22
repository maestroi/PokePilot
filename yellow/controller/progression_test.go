package controller

import "testing"

func TestYellowVermilionTrashCanCoords(t *testing.T) {
	want := [][2]uint8{
		{1, 7}, {1, 9}, {1, 11},
		{3, 7}, {3, 9}, {3, 11},
		{5, 7}, {5, 9}, {5, 11},
		{7, 7}, {7, 9}, {7, 11},
		{9, 7}, {9, 9}, {9, 11},
	}
	for i, expected := range want {
		x, y, ok := yellowVermilionTrashCanCoords(uint8(i))
		if !ok || x != expected[0] || y != expected[1] {
			t.Fatalf("index %d = (%d,%d),%v want (%d,%d),true", i, x, y, ok, expected[0], expected[1])
		}
	}
	if _, _, ok := yellowVermilionTrashCanCoords(15); ok {
		t.Fatal("index 15 unexpectedly valid")
	}
}
