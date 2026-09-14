package skill_test

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	redstarter "github.com/maestroi/pokepilot/red/starter"
	"github.com/maestroi/pokepilot/skill"
)

func TestStarterExperimentMewtwoThroughOakFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed opening run")
	}
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}

	m, err := emu.OpenCGB(romPath)
	if err != nil {
		t.Fatalf("OpenCGB: %v", err)
	}
	defer m.Close()
	base := m.ROM()
	selection, err := redstarter.Resolve("mewtwo", 4242)
	if err != nil {
		t.Fatal(err)
	}
	derived, patch, err := redstarter.Patch(base, selection)
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if len(patch.Changes) != 2 {
		t.Fatalf("patch changes = %+v, want exactly two Oak script bytes", patch.Changes)
	}
	if err := m.LoadDerivedROM(base, derived, "pokemon-red-starter-mewtwo"); err != nil {
		t.Fatalf("LoadDerivedROM: %v", err)
	}

	if _, err := skill.BootToOverworld(m); err != nil {
		t.Fatalf("BootToOverworld: %v", err)
	}
	if err := skill.GetStarter(m, m.ROM(), skill.StarterSquirtle, skill.StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("GetStarter(Mewtwo): %v", err)
	}

	var mem state.Mem
	g := state.Read(m, &mem)
	if len(g.Party.Mons) == 0 {
		t.Fatal("party is empty after Oak flow")
	}
	if got := g.Party.Mons[0].Species; got != selection.Raw {
		t.Fatalf("lead species = %#02x, want Mewtwo %#02x", got, selection.Raw)
	}

	// PokePilot still decodes through the exact verified base-ROM profile even
	// though GomeBoy is executing the two-byte derived cartridge.
	obs, err := agent.ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatalf("ObserveChecked on derived run: %v", err)
	}
	if len(obs.Party) == 0 || string(obs.Party[0].Species) != "mewtwo" {
		t.Fatalf("planner party = %+v, want Mewtwo lead", obs.Party)
	}
}
