package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// TestYellowROMOpeningThroughPokedex is the opt-in cartridge check for the
// first stretch of a normally launched Yellow run: fresh boot -> scripted
// Pikachu opening -> shared Oak Parcel/Pokedex transaction. The final fact is
// decoded by Yellow's own profile.
//
//	POKEMON_YELLOW_ROM=/path/to/yellow.gbc go test ./agent -run TestYellowROMOpeningThroughPokedex -v
func TestYellowROMOpeningThroughPokedex(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed Yellow story journey")
	}
	path := yellowROMPath(t)
	m, err := emu.OpenCGB(path)
	if err != nil {
		t.Fatalf("OpenCGB Yellow: %v", err)
	}
	defer m.Close()
	m.Pace(0)
	if _, err := skill.BootToOverworld(m); err != nil {
		t.Fatalf("BootToOverworld: %v", err)
	}

	obs, err := agent.ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatalf("observe fresh boot: %v", err)
	}
	if obs.GameID != yellowprofile.GameID {
		t.Fatalf("game = %s, want %s", obs.GameID, yellowprofile.GameID)
	}
	starter, ok := agent.DefaultStarterObjective(obs)
	if !ok || starter.Species != "pikachu" {
		t.Fatalf("fresh Yellow default starter = %+v,%v, want Pikachu", starter, ok)
	}
	if result, err := agent.Execute(m, m.ROM(), starter); err != nil {
		t.Fatalf("%s: outcome=%s: %v", starter, result.Outcome, err)
	}

	dex := agent.Objective{Kind: agent.KindProgress, Progress: gen1.ProgressPokedexAcquired}
	if result, err := agent.Execute(m, m.ROM(), dex); err != nil {
		t.Fatalf("%s: outcome=%s: %v", dex, result.Outcome, err)
	}
	obs, err = agent.ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatalf("observe after Oak's parcel: %v", err)
	}
	if !obs.Story.Has(gen1.ProgressPokedexAcquired) {
		t.Fatal("Pokedex goal completed but Yellow's story does not show the Pokedex")
	}
}

func yellowROMPath(t *testing.T) string {
	t.Helper()
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		t.Skip("POKEMON_YELLOW_ROM not set")
	}
	if filepath.IsAbs(path) {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	dir, err := os.Getwd()
	if err != nil {
		return path
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, path)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return path
		}
		dir = parent
	}
}
