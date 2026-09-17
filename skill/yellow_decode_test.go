package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	yellowstate "github.com/maestroi/pokepilot/yellow/state"
)

// TestYellowDecodeUsesYellowAddresses is the regression for the WRAM-address
// migration. Gen I decoders read a game-specific address set: Yellow shifted a
// contiguous WRAM region one byte lower than Red, so decoding a Yellow image
// with Red's free decoders returns real, plausible-looking garbage (a party of
// six and 300086 money on a fresh boot) instead of the truth. The runtime
// resolves the set per image and every decode goes through it, so a fresh
// Yellow boot must decode the game's actual start state.
//
// It boots the image rather than synthesizing RAM because the failure it guards
// is "the runtime read the wrong image's addresses", which is only visible
// against bytes the real cartridge produced.
func TestYellowDecodeUsesYellowAddresses(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	e, err := emu.OpenCGBBytes(romData)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	gs, err := BootToOverworld(e)
	if err != nil {
		t.Fatalf("BootToOverworld on Yellow: %v", err)
	}
	if gs.Player.MapID != 0x26 { // REDS_HOUSE_2F
		t.Fatalf("map = %#04x, want 0x26 (REDS_HOUSE_2F)", gs.Player.MapID)
	}
	if gs.Inventory.Money != 3000 {
		t.Fatalf("money = %d, want the fresh-game 3000; a Red-addressed decode reads 300086 here", gs.Inventory.Money)
	}
	if gs.Party.Count != 0 {
		t.Fatalf("party count = %d, want 0; a Red-addressed decode reads 6 here", gs.Party.Count)
	}

	// The address set the skill layer resolved must agree with the Yellow
	// decoder package built from yellow/sym.
	var mem state.Mem
	state.Snapshot(e, &mem)
	if got := yellowstate.Decode(&mem); got.Inventory.Money != gs.Inventory.Money || got.Party.Count != gs.Party.Count {
		t.Fatalf("runtime decode and yellow/state.Decode disagree: %+v vs %+v", gs.Inventory.Money, got.Inventory.Money)
	}
}
