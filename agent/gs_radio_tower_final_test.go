package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSRadioTowerFinaleMilestonesAreExperimental(t *testing.T) {
	for _, id := range []ProgressID{
		gsprofile.ProgressRadioTowerExecutiveM2Beaten,
		gsprofile.ProgressRadioTowerExecutiveF1Beaten,
		gsprofile.ProgressRadioTowerTakeoverSettled,
	} {
		if !gsSupportedProgress(id) {
			t.Fatalf("experimental Radio Tower stage is not executable: %s", id)
		}
	}
	for _, id := range []ProgressID{
		gsprofile.ProgressRadioTowerCleared,
		gsprofile.ProgressRisingBadgeEarned,
	} {
		if gsSupportedProgress(id) {
			t.Fatalf("unimplemented goal incorrectly offered: %s", id)
		}
	}
	if gsRadioTowerExecutiveM2X != 14 || gsRadioTowerExecutiveM2Y != 1 ||
		gsRadioTowerExecutiveF1X != 17 || gsRadioTowerExecutiveF1Y != 2 ||
		gsRadioTowerBossSceneX != 16 || gsRadioTowerBossSceneY != 5 {
		t.Fatal("Radio Tower trainer/coord scene positions drifted from retail maps")
	}
}
