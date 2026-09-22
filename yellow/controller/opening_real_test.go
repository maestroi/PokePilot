package controller

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestRealYellowPikachuOpening(t *testing.T) {
	if testing.Short() {
		t.Skip("Yellow ROM qualification is opt-in")
	}
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		path = "roms/pokemon_yellow.gb"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("POKEMON_YELLOW_ROM: %v", err)
	}
	m, err := emu.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skill.BootToOverworld(m); err != nil {
		t.Fatalf("boot: %v", err)
	}
	if err := GetPikachuStarter(m, m.ROM()); err != nil {
		t.Fatalf("Yellow opening: %v", err)
	}
	obs, err := yellowprofile.New().DecodeObservation(m, m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Party) != 1 || obs.Party[0].Species != "pikachu" {
		t.Fatalf("party = %+v, want Pikachu", obs.Party)
	}
	if !obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved) {
		t.Fatalf("opening story = %+v, want lab rival resolved", obs.Story)
	}
	if !obs.Controllable || obs.InBattle {
		t.Fatalf("opening ended at dirty boundary: controllable=%v battle=%v", obs.Controllable, obs.InBattle)
	}
}
