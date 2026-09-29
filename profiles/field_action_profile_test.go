package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestBuiltinPokemonProfilesExposeFieldMoveCapabilities(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	want := map[game.GameID]bool{
		"pokemon-red":    true,
		"pokemon-blue":   true,
		"pokemon-yellow": true, // shared Gen-I engine through the canonical view
		"pokemon-gold":   true,
		"pokemon-silver": true,
	}
	for _, profile := range registry.Profiles() {
		if !want[profile.ID()] {
			continue
		}
		if _, has := profile.(game.FieldActionProfile); !has {
			t.Errorf("%s should expose game.FieldActionProfile", profile.ID())
		}
		if _, has := profile.(game.FieldMoveProfile); !has {
			t.Errorf("%s should expose game.FieldMoveProfile", profile.ID())
		}
	}
}
