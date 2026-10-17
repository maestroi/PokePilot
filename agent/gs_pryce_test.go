package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSPryceObjectiveIsGatedUntilROMQualification(t *testing.T) {
	// B2F is now executable, but badge seven must not be advertised until
	// ROM-backed Gold/Silver checkpoints prove Rocket clearance and Pryce.
	if !gsSupportedProgress(gsprofile.ProgressRocketHideoutCleared) {
		t.Fatal("Rocket Hideout must have an executable B2F controller")
	}
	if gsSupportedProgress(gsprofile.ProgressGlacierBadgeEarned) {
		t.Fatal("Glacier Badge cannot become selectable before Rocket Hideout")
	}
}

func TestGSPryceRetailLeaderCoordinates(t *testing.T) {
	if gsPryceX != 5 || gsPryceY != 3 || gsPryceStandX != 5 || gsPryceStandY != 4 {
		t.Fatal("Pryce coordinates drifted from pokegold MahoganyGym event table")
	}
	id, err := gsOpeningMapID("MAHOGANY_GYM")
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("missing Mahogany Gym native map ID")
	}
}
