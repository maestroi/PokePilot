package skill

import "testing"

func TestTradeCenterConsoleTarget(t *testing.T) {
	tests := []struct {
		name       string
		x, y       uint8
		wantX      uint8
		wantY      uint8
		wantOK     bool
	}{
		{name: "left internal-clock seat", x: 3, y: 4, wantX: 4, wantY: 4, wantOK: true},
		{name: "right external-clock seat", x: 6, y: 4, wantX: 5, wantY: 4, wantOK: true},
		{name: "wrong row", x: 3, y: 3, wantOK: false},
		{name: "between consoles", x: 4, y: 4, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotX, gotY, gotOK := tradeCenterConsoleTarget(tt.x, tt.y)
			if gotX != tt.wantX || gotY != tt.wantY || gotOK != tt.wantOK {
				t.Fatalf("tradeCenterConsoleTarget(%d,%d) = (%d,%d,%t), want (%d,%d,%t)",
					tt.x, tt.y, gotX, gotY, gotOK, tt.wantX, tt.wantY, tt.wantOK)
			}
		})
	}
}
