package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestBuiltinGen1ProfilesExposeDexCapability(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	want := map[game.GameID]bool{
		"pokemon-red":  true,
		"pokemon-blue": true,
	}
	for _, profile := range registry.Profiles() {
		_, hasDex := profile.(game.DexProfile)
		if want[profile.ID()] && !hasDex {
			t.Errorf("%s should expose game.DexProfile", profile.ID())
		}
		if profile.ID() == "pokemon-yellow" && hasDex {
			t.Errorf("pokemon-yellow must not advertise Dex completion before its Yellow catalog is implemented")
		}
	}
}
