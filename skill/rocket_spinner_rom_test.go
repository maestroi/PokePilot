package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

func TestRocketSpinnerPlannerRoutesActualFloors(t *testing.T) {
	romData := rocketHideoutROM(t)
	tests := []struct {
		name             string
		mapID            uint8
		startX, startY   int
		warpX, warpY     int
		wantForcedAction bool
	}{
		{
			name:             "B2F entry to B3F stair",
			mapID:            rocketHideoutB2FMap,
			startX:           27,
			startY:           8,
			warpX:            21,
			warpY:            8,
			wantForcedAction: true,
		},
		{
			name:             "B3F entry to B4F stair",
			mapID:            rocketHideoutB3FMap,
			startX:           25,
			startY:           6,
			warpX:            19,
			warpY:            18,
			wantForcedAction: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := rom.ParseMap(romData, tt.mapID)
			if err != nil {
				t.Fatalf("ParseMap(%02x): %v", tt.mapID, err)
			}
			g, err := world.Build(romData, h)
			if err != nil {
				t.Fatalf("Build(%02x): %v", tt.mapID, err)
			}
			actions, err := planRocketSpinner(g.Width, g.Height, g.Walkable, tt.startX, tt.startY, tt.warpX, tt.warpY, rocketSpinnerTransitions(tt.mapID), nil)
			if err != nil {
				t.Fatalf("spinner-aware route: %v", err)
			}
			forced := 0
			for _, action := range actions {
				if action.Forced {
					forced++
				}
			}
			if tt.wantForcedAction && forced == 0 {
				t.Fatalf("route has no forced spinner action: %+v", actions)
			}
		})
	}
}
