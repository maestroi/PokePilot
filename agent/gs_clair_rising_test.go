package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSBadgeEightCoordinatesAndOwnedMaps(t *testing.T) {
	if gsIcePathWaterfallX != 31 || gsIcePathWaterfallY != 7 ||
		gsIcePathWaterfallStandX != 30 || gsIcePathWaterfallStandY != 7 ||
		gsClairX != 5 || gsClairY != 3 ||
		gsClairStandX != 5 || gsClairStandY != 4 ||
		gsDragonFangX != 35 || gsDragonFangY != 16 ||
		gsDragonFangStandX != 35 || gsDragonFangStandY != 17 {
		t.Fatal("Clair or Dragon Fang scene coordinates drifted from pokegold maps")
	}
	for _, name := range []string{
		"ROUTE_44", "ICE_PATH_1F", "ICE_PATH_B1F",
		"ICE_PATH_B2F_MAHOGANY_SIDE", "ICE_PATH_B3F",
		"ICE_PATH_B2F_BLACKTHORN_SIDE", "BLACKTHORN_CITY",
		"BLACKTHORN_POKECENTER_1F", "BLACKTHORN_GYM_1F",
		"BLACKTHORN_GYM_2F", "DRAGONS_DEN_1F", "DRAGONS_DEN_B1F",
	} {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("late Johto map %q not owned: %v", name, err)
		}
	}
	for _, milestone := range []ProgressID{
		gsprofile.ProgressIcePathWaterfallAcquired,
		gsprofile.ProgressClairDefeated,
		gsprofile.ProgressRisingBadgeEarned,
	} {
		if !gsSupportedProgress(milestone) {
			t.Fatalf("eighth badge milestone %s not executable", milestone)
		}
	}
}
