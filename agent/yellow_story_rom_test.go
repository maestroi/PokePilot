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
// first stretch of a UI-launched Yellow run: from a fresh boot, the game's
// own Starter objective must finish Yellow's opening, and the shared Oak's
// parcel beat must then prove the Pokedex from Yellow's own story facts.
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
	if !ok {
		t.Fatal("fresh Yellow boot offered no default starter")
	}
	if result, err := agent.Execute(m, m.ROM(), starter); err != nil {
		t.Fatalf("%s: outcome=%s: %v", starter, result.Outcome, err)
	}

	dex := &agent.Objective{Kind: agent.KindProgress, Progress: gen1.ProgressPokedexAcquired}
	if result, err := agent.Execute(m, m.ROM(), *dex); err != nil {
		t.Fatalf("%s: outcome=%s: %v", *dex, result.Outcome, err)
	}
	obs, err = agent.ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatalf("observe after Oak's parcel: %v", err)
	}
	if !obs.Story.Has(gen1.ProgressPokedexAcquired) {
		t.Fatal("Pokedex goal completed but Yellow's story does not show the Pokedex")
	}
}

// yellowROMPath reads POKEMON_YELLOW_ROM. go test runs in the package
// directory, so a relative path is resolved against the module root (where
// the command was most likely typed) when it does not exist as given.
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
