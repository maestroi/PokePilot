package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
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

func introNameMenuMem(current byte) *state.Mem {
	m := new(state.Mem)
	m[sym.FontLoaded] = 1
	m[sym.MaxMenuItem] = 3
	m[sym.CurrentMenuItem] = current
	// NEW NAME in Pokemon Red's font tile IDs.
	copy(m[sym.TileMap:], []byte{0x8d, 0x84, 0x96, 0x7f, 0x8d, 0x80, 0x8c, 0x84})
	return m
}

func TestBootInputSelectsFirstPresetName(t *testing.T) {
	tests := []struct {
		current byte
		want    emu.Button
	}{
		{current: 0, want: emu.Down},
		{current: 1, want: emu.A},
		{current: 2, want: emu.Up},
		{current: 3, want: emu.Up},
	}

	for _, tt := range tests {
		m := introNameMenuMem(tt.current)
		if !introNameMenu(m) {
			t.Fatalf("introNameMenu(current=%d) = false, want true", tt.current)
		}
		if got := bootInput(m, 4); got != tt.want {
			t.Fatalf("bootInput(current=%d) = %v, want %v", tt.current, got, tt.want)
		}
	}
}

func TestBootInputDoesNotTreatOrdinaryMenuAsNameEntry(t *testing.T) {
	m := new(state.Mem)
	m[sym.FontLoaded] = 1
	m[sym.MaxMenuItem] = 3
	m[sym.CurrentMenuItem] = 0
	copy(m[sym.TileMap:], []byte{0x8e, 0x80, 0x8a}) // OAK

	if introNameMenu(m) {
		t.Fatal("introNameMenu = true for ordinary intro text")
	}
	if got := bootInput(m, 4); got != emu.A {
		t.Fatalf("bootInput = %v, want A for ordinary intro text", got)
	}
}

func TestBootInputPreservesInitialStartTaps(t *testing.T) {
	m := introNameMenuMem(0)
	if got := bootInput(m, 3); got != emu.Start {
		t.Fatalf("bootInput(iteration=3) = %v, want Start", got)
	}
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
	if got := state.DecodeTiles(m.Slice(sym.PlayerName, 11)); got != "RED" {
		t.Errorf("player name = %q, want RED", got)
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
