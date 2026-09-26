package agent

import "testing"

func TestRedMartFacingTile(t *testing.T) {
	tests := []struct {
		name                   string
		px, py, clerkX, clerkY uint8
		wantX, wantY           uint8
		wantOK                 bool
	}{
		{name: "adjacent left", px: 2, py: 5, clerkX: 1, clerkY: 5, wantX: 1, wantY: 5, wantOK: true},
		{name: "adjacent above", px: 2, py: 5, clerkX: 2, clerkY: 4, wantX: 2, wantY: 4, wantOK: true},
		{name: "counter left", px: 2, py: 5, clerkX: 0, clerkY: 5, wantX: 1, wantY: 5, wantOK: true},
		{name: "counter below", px: 2, py: 5, clerkX: 2, clerkY: 7, wantX: 2, wantY: 6, wantOK: true},
		{name: "diagonal is invalid", px: 2, py: 5, clerkX: 1, clerkY: 4, wantOK: false},
		{name: "too far is invalid", px: 2, py: 5, clerkX: 5, clerkY: 5, wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotX, gotY, ok := redMartFacingTile(tc.px, tc.py, tc.clerkX, tc.clerkY)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (tile %d,%d)", ok, tc.wantOK, gotX, gotY)
			}
			if ok && (gotX != tc.wantX || gotY != tc.wantY) {
				t.Fatalf("facing tile = (%d,%d), want (%d,%d)", gotX, gotY, tc.wantX, tc.wantY)
			}
		})
	}
}
