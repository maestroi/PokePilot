package rom

import "testing"

func TestElevatorFloorTables(t *testing.T) {
	tests := []struct {
		name      string
		mapID     uint8
		panelX    uint8
		panelY    uint8
		wantMaps  []uint8
		wantWarps []uint8
	}{
		{
			name:      "celadon mart",
			mapID:     0x7F,
			panelX:    3,
			panelY:    0,
			wantMaps:  []uint8{0x7A, 0x7B, 0x7C, 0x7D, 0x88},
			wantWarps: []uint8{5, 2, 2, 2, 2},
		},
		{
			name:      "rocket hideout",
			mapID:     0xCB,
			panelX:    1,
			panelY:    1,
			wantMaps:  []uint8{0xC7, 0xC8, 0xCA},
			wantWarps: []uint8{4, 4, 2},
		},
		{
			name:      "silph co",
			mapID:     0xEC,
			panelX:    3,
			panelY:    0,
			wantMaps:  []uint8{0xB5, 0xCF, 0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xE9, 0xEA, 0xEB},
			wantWarps: []uint8{3, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := LookupElevator(tc.mapID)
			if !ok {
				t.Fatalf("LookupElevator(%02x) returned !ok", tc.mapID)
			}
			if spec.PanelX != tc.panelX || spec.PanelY != tc.panelY {
				t.Fatalf("panel=(%d,%d), want (%d,%d)", spec.PanelX, spec.PanelY, tc.panelX, tc.panelY)
			}
			if len(spec.Floors) != len(tc.wantMaps) {
				t.Fatalf("floor count=%d, want %d", len(spec.Floors), len(tc.wantMaps))
			}
			for i, floor := range spec.Floors {
				if floor.MapID != tc.wantMaps[i] || floor.DestWarpID != tc.wantWarps[i] {
					t.Errorf("floor %d = {map:%02x warp:%d}, want {map:%02x warp:%d}",
						i, floor.MapID, floor.DestWarpID, tc.wantMaps[i], tc.wantWarps[i])
				}
				_, got, index, ok := ElevatorFloorForDestination(tc.mapID, tc.wantMaps[i])
				if !ok || index != i || got != floor {
					t.Errorf("destination lookup %02x = floor=%+v index=%d ok=%v, want %+v index=%d",
						tc.wantMaps[i], got, index, ok, floor, i)
				}
			}
		})
	}
}

func TestLookupElevatorReturnsFloorCopy(t *testing.T) {
	spec, ok := LookupElevator(0x7F)
	if !ok {
		t.Fatal("Celadon elevator missing")
	}
	spec.Floors[0].MapID = 0
	again, _ := LookupElevator(0x7F)
	if again.Floors[0].MapID != 0x7A {
		t.Fatalf("caller mutated global elevator facts: first map=%02x", again.Floors[0].MapID)
	}
}
