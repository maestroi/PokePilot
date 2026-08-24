package fixture

import (
	"os"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

func checkOverworld(t *testing.T, e *emu.Emu) {
	t.Helper()
	var m state.Mem
	state.Snapshot(e, &m)
	gs := state.Decode(&m)
	if gs.Player.MapID != 0x26 {
		t.Errorf("MapID = %#04x, want 0x26", gs.Player.MapID)
	}
	if gs.Player.X != 3 || gs.Player.Y != 6 {
		t.Errorf("coords = (%d,%d), want (3,6)", gs.Player.X, gs.Player.Y)
	}
}

func TestLoadGeneratesAndCaches(t *testing.T) {
	e1 := Load(t, "reds_bedroom")
	if _, err := os.Stat(fixturePath("reds_bedroom")); err != nil {
		t.Fatalf("fixture file not created after first Load: %v", err)
	}
	checkOverworld(t, e1)

	e2 := Load(t, "reds_bedroom")
	checkOverworld(t, e2)
}

func TestLoadRejectsPoisonedFixture(t *testing.T) {
	rom := os.Getenv("POKEMON_RED_ROM")
	if rom == "" {
		t.Skip("POKEMON_RED_ROM not set; cannot write poisoned fixture")
	}
	// A state stepped only 60 frames is still on the title screen with zero
	// map dimensions: deliberately invalid fixture content.
	e, err := emu.Open(rom)
	if err != nil {
		t.Fatalf("emu.Open: %v", err)
	}
	e.StepFrames(60)
	b, err := e.SaveState()
	if err != nil {
		e.Close()
		t.Fatalf("SaveState: %v", err)
	}
	e.Close()
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", Dir, err)
	}
	path := fixturePath("reds_bedroom")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write poisoned fixture: %v", err)
	}

	loaded := Load(t, "reds_bedroom")
	var m state.Mem
	state.Snapshot(loaded, &m)
	if !state.Controllable(&m) {
		t.Fatal("Load returned a non-controllable state; poisoned fixture was trusted instead of regenerated")
	}
	checkOverworld(t, loaded)
}

func TestFixtureIsFast(t *testing.T) {
	Load(t, "reds_bedroom")
	start := time.Now()
	Load(t, "reds_bedroom")
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("second Load took %v, want < 2s (cache should make it fast)", d)
	}
}

func TestFixtureStateIsControllable(t *testing.T) {
	e := Load(t, "reds_bedroom")
	var m state.Mem
	state.Snapshot(e, &m)
	if !state.Controllable(&m) {
		t.Error("Controllable = false, want true")
	}
}
