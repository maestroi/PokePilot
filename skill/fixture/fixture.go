// Package fixture caches expensive emulator boot sequences as save states so
// tests start from a deterministic, millisecond-fast state instead of replay
//ing thousands of frames.
//
// Fixtures are derived from a commercial ROM, so they are generated on demand
// from the ROM named by POKEMON_RED_ROM and cached under Dir; they are never
// committed.
//
// It lives in its own package (not inside skill) because it imports skill for
// BootToOverworld, and skill's tests import it.
package fixture

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// Dir is where generated fixtures are cached. It is gitignored.
const Dir = "testdata/fixtures"

// fixtureVersion is embedded in fixture filenames. Bump it whenever the boot
// sequence or the definition of a valid state changes: a new version
// invalidates every stale fixture at once.
const fixtureVersion = 2

// builders maps a fixture name to the function that produces it from a
// freshly booted emulator.
var builders = map[string]func(*emu.Emu) error{}

// Register associates a fixture name with the function that produces it
// from a freshly booted emulator.
func Register(name string, build func(*emu.Emu) error) {
	builders[name] = build
}

func init() {
	// reds_bedroom: the state immediately after BootToOverworld.
	Register("reds_bedroom", func(*emu.Emu) error { return nil })
}

// fixturePath returns the cache path for a named fixture at the current
// fixtureVersion.
func fixturePath(name string) string {
	return filepath.Join(Dir, fmt.Sprintf("%s.v%d.state", name, fixtureVersion))
}

// validateState snapshots the emulator and reports the decoded game state
// plus whether it is a valid fixture state (controllable overworld).
func validateState(e *emu.Emu) (state.GameState, bool) {
	var m state.Mem
	state.Snapshot(e, &m)
	return state.Decode(&m), state.Controllable(&m)
}

// Load returns an emulator restored to the named fixture, generating and
// caching the fixture first if it does not exist. It calls t.Skip when the
// POKEMON_RED_ROM environment variable is not set.
//
// A cached fixture is validated before being trusted: if it fails validation
// (state.Controllable is false) the file is deleted and the fixture is
// regenerated from scratch, so a poisoned cache entry can never be returned.
func Load(t *testing.T, name string) *emu.Emu {
	t.Helper()
	rom := os.Getenv("POKEMON_RED_ROM")
	if rom == "" {
		t.Skip("POKEMON_RED_ROM not set; cannot generate or load fixture " + name)
	}

	path := fixturePath(name)
	if b, err := os.ReadFile(path); err == nil {
		e, err := emu.Open(rom)
		if err != nil {
			t.Fatalf("fixture %s: emu.Open: %v", name, err)
		}
		if err := e.LoadState(b); err != nil {
			e.Close()
			t.Fatalf("fixture %s: LoadState: %v", name, err)
		}
		gs, ok := validateState(e)
		if ok {
			t.Cleanup(func() { e.Close() })
			return e
		}
		e.Close()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatalf("fixture %s: remove poisoned %s: %v", name, path, err)
		}
		t.Logf("fixture %s: cached state failed validation (MapID=%#04x, controllable=false); deleted %s and regenerating", name, gs.Player.MapID, path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("fixture %s: read %s: %v", name, path, err)
	}

	build, ok := builders[name]
	if !ok {
		t.Fatalf("fixture %s: not registered", name)
	}
	e, err := emu.Open(rom)
	if err != nil {
		t.Fatalf("fixture %s: emu.Open: %v", name, err)
	}
	if _, err := skill.BootToOverworld(e); err != nil {
		e.Close()
		t.Fatalf("fixture %s: boot: %v", name, err)
	}
	if err := build(e); err != nil {
		e.Close()
		t.Fatalf("fixture %s: build: %v", name, err)
	}
	gs, ok := validateState(e)
	if !ok {
		e.Close()
		t.Fatalf("fixture %s: generated state failed validation: MapID=%#04x X=%d Y=%d CurMapWidth=%d CurMapHeight=%d",
			name, gs.Player.MapID, gs.Player.X, gs.Player.Y, gs.World.Width, gs.World.Height)
	}
	b, err := e.SaveState()
	if err != nil {
		e.Close()
		t.Fatalf("fixture %s: SaveState: %v", name, err)
	}
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		e.Close()
		t.Fatalf("fixture %s: mkdir %s: %v", name, Dir, err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		e.Close()
		t.Fatalf("fixture %s: write %s: %v", name, path, err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}
