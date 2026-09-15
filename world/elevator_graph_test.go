package world

import "testing"

func TestBuildGraphModelsSelectableElevatorFloors(t *testing.T) {
	g := loadGraph(t)
	tests := []struct {
		name      string
		from      uint8
		warpX     uint8
		warpY     uint8
		want      []uint8
		forbidden []uint8
	}{
		{
			name:  "celadon mart",
			from:  0x7F,
			warpX: 1,
			warpY: 3,
			want:  []uint8{0x7A, 0x7B, 0x7C, 0x7D, 0x88},
		},
		{
			name:      "rocket hideout",
			from:      0xCB,
			warpX:     2,
			warpY:     1,
			want:      []uint8{0xC7, 0xC8, 0xCA},
			forbidden: []uint8{0xC9},
		},
		{
			name:  "silph co",
			from:  0xEC,
			warpX: 1,
			warpY: 3,
			want:  []uint8{0xB5, 0xCF, 0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xE9, 0xEA, 0xEB},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, to := range tc.want {
				if !hasWarpEdge(g, tc.from, to, tc.warpX, tc.warpY) {
					t.Errorf("elevator %02x missing warp (%d,%d) -> %02x", tc.from, tc.warpX, tc.warpY, to)
				}
			}
			for _, to := range tc.forbidden {
				if hasWarpEdge(g, tc.from, to, tc.warpX, tc.warpY) {
					t.Errorf("elevator %02x unexpectedly advertises floor %02x", tc.from, to)
				}
			}
		})
	}
}

func TestBuildGraphElevatorEdgesResolveLandingComponents(t *testing.T) {
	g := loadGraph(t)
	want := Edge{Kind: EdgeWarp, From: 0x7F, To: 0x7D, WarpX: 1, WarpY: 3}
	found := false
	for _, edge := range g.Edges[want.From] {
		if edge != want {
			continue
		}
		found = true
		if len(g.exitComps[edge]) == 0 {
			t.Errorf("Celadon elevator edge has no source component: %+v", edge)
		}
		if len(g.entryComps[edge]) == 0 {
			t.Errorf("Celadon elevator edge has no 4F landing component: %+v", edge)
		}
	}
	if !found {
		t.Fatalf("Celadon elevator edge not found: %+v", want)
	}
}
