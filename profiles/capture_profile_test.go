package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestBuiltinGen1ProfilesExposeCaptureAndInventoryCapabilities(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range registry.Profiles() {
		_, capture := profile.(game.CaptureProfile)
		_, inventory := profile.(game.InventoryProfile)
		switch profile.ID() {
		case "pokemon-red", "pokemon-blue":
			if !capture || !inventory {
				t.Errorf("%s capture=%v inventory=%v, want both true", profile.ID(), capture, inventory)
			}
		case "pokemon-yellow":
			if capture || inventory {
				t.Errorf("pokemon-yellow must not advertise unvalidated capture/inventory capabilities")
			}
		}
	}
}
