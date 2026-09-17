package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// TestYellowStarterFixture pins the end of the Yellow opening: the fixture is
// the moment Oak hands the player the starter, so it carries a populated party
// on Yellow RAM. It is the smallest deterministic form of "the whole Yellow
// stack agrees": a state the real cartridge produced, decoded through the
// Yellow address set, with a party a Red-addressed decode cannot read.
//
// The fixture was captured by driving the real image from power-on through the
// Oak sequence: bedroom -> 1F -> Pallet Town -> north exit (Oak appears) ->
// wild Pikachu cutscene -> Oak's Lab -> rival speeches -> starter ball. That
// drive lives in the docs; this test loads its endpoint instead of replaying
// it, because the drive is minutes and this is milliseconds, and the drive's
// RNG (rDIV-seeded) is not what this test pins.
func TestYellowStarterFixture(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	st, err := os.ReadFile("failure/yellow_starter.state")
	if err != nil {
		t.Skip("yellow_starter.state not available")
	}
	e, err := emu.OpenCGBBytes(romData)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.LoadState(st); err != nil {
		t.Fatal(err)
	}

	var mem state.Mem
	a := AddressesForROM(romData)
	state.Snapshot(e, &mem)
	gs := a.Decode(&mem)

	if gs.Party.Count == 0 {
		t.Fatal("party count = 0; the starter fixture has no mon")
	}
	mon := gs.Party.Mons[0]
	// Species 84 is Pikachu, level 5: the starter Yellow awards. A
	// Red-addressed decode of this RAM reads the species from one byte earlier
	// and gets a different, wrong mon, so this is the live invariant.
	const pikachuID, starterLevel = 84, 5
	if mon.Species != pikachuID {
		t.Fatalf("lead species = %d, want %d (Pikachu); a Red-addressed decode lands one byte into the party struct", mon.Species, pikachuID)
	}
	if mon.Level != starterLevel {
		t.Fatalf("lead level = %d, want %d", mon.Level, starterLevel)
	}
	if mon.HP == 0 || mon.MaxHP == 0 {
		t.Fatalf("lead HP = %d/%d, want a live mon", mon.HP, mon.MaxHP)
	}
	t.Logf("Yellow starter: species=%d lvl=%d hp=%d/%d", mon.Species, mon.Level, mon.HP, mon.MaxHP)

	// The fixture must be Yellow, not Red: a Red decode of the same RAM must
	// disagree, which is what proves this fixture exercises the Yellow set.
	redGS := redWram().Decode(&mem)
	if redGS.Party.Mons[0].Species == mon.Species && redGS.Party.Mons[0].Level == mon.Level {
		t.Fatal("the Red decode agrees with the Yellow decode on this RAM; the fixture is not discriminating between address sets")
	}
}
