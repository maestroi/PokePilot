package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestBuiltinGen1ProfilesExposeListMenuCapability(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	want := map[game.GameID]bool{
		"pokemon-red":    true,
		"pokemon-blue":   true,
		"pokemon-yellow": true, // shared Gen-I engine through the canonical view
	}
	for _, profile := range registry.Profiles() {
		_, hasListMenu := profile.(game.ListMenuProfile)
		if want[profile.ID()] && !hasListMenu {
			t.Errorf("%s should expose game.ListMenuProfile", profile.ID())
		}
	}
}
