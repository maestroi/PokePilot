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
	// Oak's name menus never set wFontLoaded; detection must not require it.
	m[sym.MaxMenuItem] = 3
	m[sym.CurrentMenuItem] = current
	// NEW NAME in Pokemon Red's font tile IDs.
	copy(m[sym.TileMap:], []byte{0x8d, 0x84, 0x96, 0x7f, 0x8d, 0x80, 0x8c, 0x84})
	return m
}

func TestIntroNameMenuDoesNotRequireFontLoaded(t *testing.T) {
	m := introNameMenuMem(0)
	if m[sym.FontLoaded] != 0 {
		t.Fatalf("fixture FontLoaded = %d, want 0", m[sym.FontLoaded])
	}
	if !introNameMenu(m, redWram()) {
		t.Fatal("introNameMenu = false when NEW NAME is on screen with FontLoaded=0")
	}
}

func TestBootInputSelectsAnimePresetNames(t *testing.T) {
	tests := []struct {
		current byte
		want    emu.Button
	}{
		{current: 0, want: emu.Down},
		{current: 1, want: emu.Down},
		{current: 2, want: emu.A},
		{current: 3, want: emu.Up},
	}

	for _, tt := range tests {
		m := introNameMenuMem(tt.current)
		if !introNameMenu(m, redWram()) {
			t.Fatalf("introNameMenu(current=%d, redWram()) = false, want true", tt.current)
		}
		if got := bootInput(m, redWram(), 4); got != tt.want {
			t.Fatalf("bootInput(current=%d, redWram()) = %v, want %v", tt.current, got, tt.want)
		}
	}
}

func TestBootInputDoesNotTreatOrdinaryMenuAsNameEntry(t *testing.T) {
	m := new(state.Mem)
	m[sym.FontLoaded] = 1
	m[sym.MaxMenuItem] = 3
	m[sym.CurrentMenuItem] = 0
	copy(m[sym.TileMap:], []byte{0x8e, 0x80, 0x8a}) // OAK

	if introNameMenu(m, redWram()) {
		t.Fatal("introNameMenu = true for ordinary intro text")
	}
	if got := bootInput(m, redWram(), 4); got != emu.A {
		t.Fatalf("bootInput = %v, want A for ordinary intro text", got)
	}
}

func TestDecodeBootedOverworldRequiresAshAndGary(t *testing.T) {
	m := new(state.Mem)
	copy(m[sym.PlayerName:], []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x50})
	if _, err := decodeBootedOverworld(m, redWram()); err == nil {
		t.Fatal("decodeBootedOverworld(AAAAAAA, redWram()) = nil, want error")
	}

	copy(m[sym.PlayerName:], []byte{0x80, 0x92, 0x87, 0x50}) // ASH
	if _, err := decodeBootedOverworld(m, redWram()); err == nil {
		t.Fatal("decodeBootedOverworld(ASH, empty rival, redWram()) = nil, want error")
	}

	copy(m[sym.RivalName:], []byte{0x86, 0x80, 0x91, 0x98, 0x50}) // GARY
	if _, err := decodeBootedOverworld(m, redWram()); err != nil {
		t.Fatalf("decodeBootedOverworld(ASH, GARY, redWram()) = %v, want nil", err)
	}
}

func TestBootInputPreservesInitialStartTaps(t *testing.T) {
	m := introNameMenuMem(0)
	if got := bootInput(m, redWram(), 3); got != emu.Start {
		t.Fatalf("bootInput(iteration=3, redWram()) = %v, want Start", got)
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
	if !redWram().Controllable(&m) {
		t.Errorf("Controllable = false, want true")
	}
	if got := state.DecodeName(m.Slice(sym.PlayerName, 11)); got != "ASH" {
		t.Errorf("player name = %q, want ASH", got)
	}
	if got := state.DecodeName(m.Slice(sym.RivalName, 11)); got != "GARY" {
		t.Errorf("rival name = %q, want GARY", got)
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
