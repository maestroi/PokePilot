package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
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

func openEmuCGB(t *testing.T) *emu.Emu {
	t.Helper()
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	e, err := emu.OpenCGB(path)
	if err != nil {
		t.Fatalf("emu.OpenCGB: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

func TestBootInputSelectsSecondPreset(t *testing.T) {
	tests := []struct {
		current byte
		want    emu.Button
	}{
		{0, emu.Down},
		{1, emu.Down},
		{2, emu.A},
		{3, emu.Up},
	}
	for _, tt := range tests {
		state := game.BootState{NameMenu: true, CurrentMenuItem: tt.current}
		if got, press := bootInput(state, 4); !press || got != tt.want {
			t.Fatalf("bootInput(current=%d) = (%v,%v), want (%v,true)", tt.current, got, press, tt.want)
		}
	}
}

func TestBootInputUsesAForOrdinaryIntroState(t *testing.T) {
	if got, press := bootInput(game.BootState{NameMenu: false, MaxMenuItem: 3}, 4); !press || got != emu.A {
		t.Fatalf("bootInput = (%v,%v), want (A,true)", got, press)
	}
}

func TestBootInputPreservesInitialStartTaps(t *testing.T) {
	if got, press := bootInput(game.BootState{NameMenu: true}, 3); !press || got != emu.Start {
		t.Fatalf("bootInput(iteration=3) = (%v,%v), want (Start,true)", got, press)
	}
}

func TestVerifyBootedOverworldRequiresSelectedPresets(t *testing.T) {
	state := game.BootState{PlayerName: "AAAAAAA", RivalName: "GARY"}
	if err := verifyBootedOverworld(state, []string{"ASH", "GARY"}); err == nil {
		t.Fatal("verifyBootedOverworld accepted wrong player name")
	}
	state.PlayerName = "ASH"
	if err := verifyBootedOverworld(state, []string{"ASH", "GARY"}); err != nil {
		t.Fatalf("verifyBootedOverworld = %v", err)
	}
}

func TestBootToOverworld(t *testing.T) {
	e := openEmu(t)
	obs, err := BootToOverworld(e)
	if err != nil {
		t.Fatalf("BootToOverworld: %v", err)
	}
	if obs.NativeMapID != 0x26 {
		t.Errorf("MapID = %#04x, want 0x26", obs.NativeMapID)
	}
	if obs.X != 3 || obs.Y != 6 {
		t.Errorf("coords = (%d,%d), want (3,6)", obs.X, obs.Y)
	}
	if !obs.Controllable {
		t.Error("Controllable = false, want true")
	}
}

func TestBootIsRepeatable(t *testing.T) {
	e1 := openEmu(t)
	obs1, err := BootToOverworld(e1)
	if err != nil {
		t.Fatalf("boot 1: %v", err)
	}
	e2 := openEmu(t)
	obs2, err := BootToOverworld(e2)
	if err != nil {
		t.Fatalf("boot 2: %v", err)
	}
	if obs1.NativeMapID != obs2.NativeMapID || obs1.X != obs2.X || obs1.Y != obs2.Y {
		t.Errorf("boot not repeatable: (%#04x,%d,%d) vs (%#04x,%d,%d)",
			obs1.NativeMapID, obs1.X, obs1.Y,
			obs2.NativeMapID, obs2.X, obs2.Y)
	}
}

func TestBootInputHonorsProfileSemanticInput(t *testing.T) {
	tests := []struct {
		input game.BootInput
		want  emu.Button
		press bool
	}{
		{game.BootInputConfirm, emu.A, true},
		{game.BootInputStart, emu.Start, true},
		{game.BootInputUp, emu.Up, true},
		{game.BootInputDown, emu.Down, true},
		{game.BootInputWait, 0, false},
	}
	for _, tt := range tests {
		got, press := bootInput(game.BootState{NextInput: tt.input}, 0)
		if got != tt.want || press != tt.press {
			t.Fatalf("bootInput(%d) = (%v,%v), want (%v,%v)", tt.input, got, press, tt.want, tt.press)
		}
	}
}

func TestBootGoldToBedroom(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	e, err := emu.Open(path)
	if err != nil {
		t.Fatalf("emu.Open Gold: %v", err)
	}
	t.Cleanup(func() { e.Close() })

	obs, err := BootToOverworld(e)
	if err != nil {
		t.Fatalf("BootToOverworld Gold: %v", err)
	}
	if obs.NativeMapID != 0x1807 {
		t.Fatalf("Gold map = %#04x, want PLAYERS_HOUSE_2F (0x1807)", obs.NativeMapID)
	}
	if !obs.Controllable {
		t.Fatal("Gold bedroom is not controllable")
	}
}

func TestBootSilverToBedroom(t *testing.T) {
	path := os.Getenv("POKEMON_SILVER_ROM")
	if path == "" {
		t.Skip("POKEMON_SILVER_ROM not set")
	}
	e, err := emu.Open(path)
	if err != nil {
		t.Fatalf("emu.Open Silver: %v", err)
	}
	t.Cleanup(func() { e.Close() })

	obs, err := BootToOverworld(e)
	if err != nil {
		t.Fatalf("BootToOverworld Silver: %v", err)
	}
	if obs.NativeMapID != 0x1807 {
		t.Fatalf("Silver map = %#04x, want PLAYERS_HOUSE_2F (0x1807)", obs.NativeMapID)
	}
	if !obs.Controllable {
		t.Fatal("Silver bedroom is not controllable")
	}
}
