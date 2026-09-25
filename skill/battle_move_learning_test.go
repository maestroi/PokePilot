package skill

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestHardIgnoreNaturalMove(t *testing.T) {
	tests := []struct {
		name       string
		move       rom.Move
		wantIgnore bool
		wantReason string
	}{
		{name: "Splash", move: rom.Move{Effect: 0x55}, wantIgnore: true, wantReason: "no battle effect"},
		{name: "Focus Energy", move: rom.Move{Effect: 0x2f}, wantIgnore: true, wantReason: "broken in Gen 1"},
		{name: "Sleep", move: rom.Move{Effect: 0x20}, wantIgnore: false},
		{name: "Damaging variant stays eligible", move: rom.Move{Power: 40, Effect: 0x55}, wantIgnore: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ignore := hardIgnoreNaturalMove(tt.move)
			if ignore != tt.wantIgnore {
				t.Fatalf("hardIgnoreNaturalMove(%+v) ignore=%v, want %v (reason %q)", tt.move, ignore, tt.wantIgnore, reason)
			}
			if tt.wantReason != "" && !strings.Contains(reason, tt.wantReason) {
				t.Fatalf("hardIgnoreNaturalMove(%+v) reason=%q, want substring %q", tt.move, reason, tt.wantReason)
			}
		})
	}
}
