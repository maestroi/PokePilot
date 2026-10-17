package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSRadioTowerFirstGuardedControllerStaysOutOfPlanner(t *testing.T) {
	if gsRadioTowerFakeDirectorSceneX != 0 || gsRadioTowerFakeDirectorSceneY != 3 {
		t.Fatal("fake director 5F scene coordinates drifted from retail event table")
	}
	for _, name := range []string{
		"RADIO_TOWER_1F", "RADIO_TOWER_2F", "RADIO_TOWER_3F",
		"RADIO_TOWER_4F", "RADIO_TOWER_5F",
	} {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("Radio Tower approach map %s is not safely owned: %v", name, err)
		}
	}
	for _, name := range []string{
		"GOLDENROD_UNDERGROUND",
		"GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES",
		"GOLDENROD_UNDERGROUND_WAREHOUSE",
	} {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("Radio Tower basement story map %s should be owned: %v", name, err)
		}
	}
	if gsSupportedProgress(gsprofile.ProgressRadioTowerFakeDirectorBeaten) ||
		gsSupportedProgress(gsprofile.ProgressRadioTowerCleared) ||
		gsSupportedProgress(gsprofile.ProgressGlacierBadgeEarned) ||
		gsSupportedProgress(gsprofile.ProgressRisingBadgeEarned) {
		t.Fatal("unqualified badge 7/8 story goals must stay outside production planner")
	}
}
