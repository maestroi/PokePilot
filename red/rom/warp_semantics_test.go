package rom

import "testing"

func TestIsInertWarp(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mapID, x, y uint8
	}{
		{name: "celadon inaccessible mart entry", mapID: 0x06, x: 39, y: 19},
		{name: "silph 1f inaccessible stair", mapID: 0xb5, x: 16, y: 10},
		{name: "silph 11f inaccessible pad", mapID: 0xeb, x: 5, y: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !IsInertWarp(tc.mapID, tc.x, tc.y) {
				t.Fatalf("IsInertWarp(%#02x,%d,%d) = false", tc.mapID, tc.x, tc.y)
			}
		})
	}

	if IsInertWarp(0xeb, 4, 5) {
		t.Fatal("ordinary Silph 11F tile was classified inert")
	}
}
