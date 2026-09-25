package profiles

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestBuiltinGen1ProfilesExposeBattleMenuCapability(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	want := map[game.GameID]bool{
		"pokemon-red":  true,
		"pokemon-blue": true,
	}
	for _, profile := range registry.Profiles() {
		_, hasBattleMenu := profile.(game.BattleMenuProfile)
		if want[profile.ID()] && !hasBattleMenu {
			t.Errorf("%s should expose game.BattleMenuProfile", profile.ID())
		}
		if profile.ID() == "pokemon-yellow" && hasBattleMenu {
			t.Errorf("pokemon-yellow must not advertise battle-menu semantics before its battle adapter exists")
		}
	}
}
