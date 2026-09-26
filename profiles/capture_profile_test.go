package profiles

import (
	"testing"

	blueprofile "github.com/maestroi/pokepilot/blue/profile"
	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestBuiltinGen1ProfilesExposeCaptureAndInventoryCapabilities(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	gen1 := map[game.GameID]bool{
		redprofile.GameID:    true,
		blueprofile.GameID:   true,
		yellowprofile.GameID: true,
	}
	seen := map[game.GameID]bool{}
	for _, profile := range registry.Profiles() {
		if !gen1[profile.ID()] {
			continue
		}
		seen[profile.ID()] = true
		_, capture := profile.(game.CaptureProfile)
		_, inventory := profile.(game.InventoryProfile)
		// Yellow serves the shared Gen-I engine through its canonical view.
		if !capture || !inventory {
			t.Errorf("%s capture=%v inventory=%v, want both true", profile.ID(), capture, inventory)
		}
	}
	for id := range gen1 {
		if !seen[id] {
			t.Errorf("built-in registry missing Gen-I profile %s", id)
		}
	}
}
