package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestBuiltinGen1ProfilesExposePartyMenuCapability(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	want := map[game.GameID]bool{
		"pokemon-red":  true,
		"pokemon-blue": true,
	}
	for _, profile := range registry.Profiles() {
		_, hasPartyMenu := profile.(game.PartyMenuProfile)
		if want[profile.ID()] && !hasPartyMenu {
			t.Errorf("%s should expose game.PartyMenuProfile", profile.ID())
		}
		if profile.ID() == "pokemon-yellow" && hasPartyMenu {
			t.Errorf("pokemon-yellow must not advertise party-menu semantics before its battle adapter exists")
		}
	}
}
