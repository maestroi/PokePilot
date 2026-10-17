package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSPryceObjectiveIsGatedUntilRocketHideoutIsExecutable(t *testing.T) {
	// The controller is implemented but must not be offered as an unsupported
	// mainline objective before the real Rocket Hideout clear is owned.
	if gsSupportedProgress(gsprofile.ProgressRocketHideoutCleared) {
		t.Fatal("Rocket Hideout must stay unsupported until its controller lands")
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
