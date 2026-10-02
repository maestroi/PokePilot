package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

// TestGen2ProfilesDeclareTheirMenuPressCadence pins the registry wiring for
// game.MenuPressTimingProfile. Gold/Silver menus swallow a 3-frame press that
// Gen-I menus accept, so the reusable menu driver has to learn the longer hold
// from the profile instead of from a frame count tuned for Red.
func TestGen2ProfilesDeclareTheirMenuPressCadence(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	want := map[game.GameID]bool{
		"pokemon-gold":   true,
		"pokemon-silver": true,
	}
	for _, profile := range registry.Profiles() {
		timing, hasTiming := profile.(game.MenuPressTimingProfile)
		if want[profile.ID()] && !hasTiming {
			t.Errorf("%s should expose game.MenuPressTimingProfile", profile.ID())
			continue
		}
		if !hasTiming {
			continue
		}
		// Below 4 frames the press is dropped before the menu polls; the
		// per-profile window test in gs/profile owns the upper bound.
		if hold := timing.MenuPressHoldFrames(); hold < 4 {
			t.Errorf("%s declares a %d-frame menu press hold; shorter presses are dropped", profile.ID(), hold)
		}
	}
}
