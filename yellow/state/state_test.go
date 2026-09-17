package state

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
)

// bootState is a saved emulator state reached by booting Yellow to a
// controllable overworld (REDS_HOUSE_2F). It is skipped when absent.
const bootState = "../../skill/failure/yellow_overworld.state"

// loadStateOrSkip boots the Yellow ROM and restores a gomeboy serialized
// state, returning the decoded RAM. State files are emulator snapshots, not
// raw memory dumps, so they must go through the emulator.
func loadStateOrSkip(t *testing.T) *Mem {
	t.Helper()
	rom, err := os.ReadFile("../../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	state, err := os.ReadFile(bootState)
	if err != nil {
		t.Skipf("boot state %s not available", bootState)
	}
	e, err := emu.OpenCGBBytes(rom)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	if err := e.LoadState(state); err != nil {
		t.Fatalf("load state: %v", err)
	}
	var mem Mem
	e.PeekInto(0, mem[:])
	return &mem
}

// TestDecodeBootState decodes the real booted-overworld state and asserts the
// facts the boot left behind: the player is in REDS_HOUSE_2F at the known
// tile, holding the starting money, with no party yet (Pikachu is received
// after leaving the room). This is the positive postcondition that the symbol
// table is right, not merely "no error".
func TestDecodeBootState(t *testing.T) {
	mem := loadStateOrSkip(t)
	gs := Decode(mem)

	if gs.Player.MapID != 0x26 {
		t.Errorf("map id = 0x%02x, want 0x26 (REDS_HOUSE_2F)", gs.Player.MapID)
	}
	t.Logf("player: map=0x%02x x=%d y=%d facing=%s walking=%v",
		gs.Player.MapID, gs.Player.X, gs.Player.Y, gs.Player.Facing, gs.Player.Walking)

	if gs.Inventory.Money != 3000 {
		t.Errorf("money = %d, want 3000 (Yellow's starting money)", gs.Inventory.Money)
	}
	if gs.Party.Count != 0 {
		t.Errorf("party count = %d, want 0 (starter comes after the room)", gs.Party.Count)
	}
	if gs.Battle != nil {
		t.Error("battle is set on the overworld")
	}
	if !Controllable(mem) {
		t.Error("Controllable is false on the booted overworld")
	}
	if gs.World.Tileset != 4 { // REDS_HOUSE_2 tileset
		t.Errorf("tileset = %d, want 4", gs.World.Tileset)
	}
}

// TestMapName pins the two ids that differ from Red plus an unused slot.
func TestMapName(t *testing.T) {
	cases := []struct {
		id   uint8
		want string
	}{
		{0x00, "PALLET_TOWN"},
		{0x3F, "CERULEAN_MELANIES_HOUSE"},
		{0xF8, "SUMMER_BEACH_HOUSE"},
		{0x0B, "UNUSED_MAP_0B"},
	}
	for _, c := range cases {
		if got := MapName(c.id); got != c.want {
			t.Errorf("MapName(0x%02x) = %q, want %q", c.id, got, c.want)
		}
	}
}
