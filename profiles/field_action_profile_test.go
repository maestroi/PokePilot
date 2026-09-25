package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestBuiltinGen1ProfilesExposeFieldActionCapability(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	want := map[game.GameID]bool{
		"pokemon-red":  true,
		"pokemon-blue": true,
	}
	for _, profile := range registry.Profiles() {
		_, has := profile.(game.FieldActionProfile)
		if want[profile.ID()] && !has {
			t.Errorf("%s should expose game.FieldActionProfile", profile.ID())
		}
		if profile.ID() == "pokemon-yellow" && has {
			t.Errorf("pokemon-yellow must not advertise field-action semantics before its adapter is validated")
		}
	}
}
