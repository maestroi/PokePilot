package skill

import "testing"

func TestVermilionTrashCanCoords(t *testing.T) {
	want := [][2]uint8{
		{1, 7}, {1, 9}, {1, 11},
		{3, 7}, {3, 9}, {3, 11},
		{5, 7}, {5, 9}, {5, 11},
		{7, 7}, {7, 9}, {7, 11},
		{9, 7}, {9, 9}, {9, 11},
	}
	for i, xy := range want {
		x, y, ok := vermilionTrashCanCoords(uint8(i))
		if !ok || x != xy[0] || y != xy[1] {
			t.Fatalf("can %d = (%d,%d,%v), want (%d,%d,true)", i, x, y, ok, xy[0], xy[1])
		}
	}
	if _, _, ok := vermilionTrashCanCoords(15); ok {
		t.Fatal("can 15 unexpectedly accepted")
	}
}
