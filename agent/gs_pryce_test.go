package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSPryceObjectiveCanBeAttemptedExperimentally(t *testing.T) {
	// Opt-in runs can try Pryce; the controller still requires the
	// cartridge's actual Rocket Hideout clear and Glacier Badge bits.
	if !gsSupportedProgress(gsprofile.ProgressRocketHideoutCleared) {
		t.Fatal("Rocket Hideout must have an executable B2F controller")
	}
	if !gsSupportedProgress(gsprofile.ProgressGlacierBadgeEarned) {
		t.Fatal("opt-in Glacier Badge controller must be executable")
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
