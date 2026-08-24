package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

func openEmu(t *testing.T) *emu.Emu {
	t.Helper()
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	e, err := emu.Open(path)
	if err != nil {
		t.Fatalf("emu.Open: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

func TestBootToOverworld(t *testing.T) {
	e := openEmu(t)
	gs, err := BootToOverworld(e)
	if err != nil {
		t.Fatalf("BootToOverworld: %v", err)
	}
	if gs.Player.MapID != 0x26 {
		t.Errorf("MapID = %#04x, want 0x26", gs.Player.MapID)
	}
	if gs.Player.X != 3 || gs.Player.Y != 6 {
		t.Errorf("coords = (%d,%d), want (3,6)", gs.Player.X, gs.Player.Y)
	}
	if gs.World.Width != 4 || gs.World.Height != 4 {
		t.Errorf("map dimensions = (%d,%d), want (4,4)", gs.World.Width, gs.World.Height)
	}
	var m state.Mem
	state.Snapshot(e, &m)
	if !state.Controllable(&m) {
		t.Errorf("Controllable = false, want true")
	}
}

func TestBootIsRepeatable(t *testing.T) {
	e1 := openEmu(t)
	gs1, err := BootToOverworld(e1)
	if err != nil {
		t.Fatalf("boot 1: %v", err)
	}
	e2 := openEmu(t)
	gs2, err := BootToOverworld(e2)
	if err != nil {
		t.Fatalf("boot 2: %v", err)
	}
	if gs1.Player.MapID != gs2.Player.MapID ||
		gs1.Player.X != gs2.Player.X ||
		gs1.Player.Y != gs2.Player.Y {
		t.Errorf("boot not repeatable: (%#04x,%d,%d) vs (%#04x,%d,%d)",
			gs1.Player.MapID, gs1.Player.X, gs1.Player.Y,
			gs2.Player.MapID, gs2.Player.X, gs2.Player.Y)
	}
}
