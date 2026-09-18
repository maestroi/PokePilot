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
		if got := bootInput(state, 4); got != tt.want {
			t.Fatalf("bootInput(current=%d) = %v, want %v", tt.current, got, tt.want)
		}
	}
}

func TestBootInputUsesAForOrdinaryIntroState(t *testing.T) {
	if got := bootInput(game.BootState{NameMenu: false, MaxMenuItem: 3}, 4); got != emu.A {
		t.Fatalf("bootInput = %v, want A", got)
	}
}

func TestBootInputPreservesInitialStartTaps(t *testing.T) {
	if got := bootInput(game.BootState{NameMenu: true}, 3); got != emu.Start {
		t.Fatalf("bootInput(iteration=3) = %v, want Start", got)
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
