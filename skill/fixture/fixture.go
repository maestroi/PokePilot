// Package fixture caches expensive emulator boot sequences as save states so
// tests start from a deterministic, millisecond-fast state instead of replay
// ing thousands of frames.
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
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// Dir is where generated fixtures are cached. It is gitignored.
const Dir = "testdata/fixtures"

// FailureDir is where a failing test's final save state is dumped. It is
// gitignored. The state is the only reproducible record a journey test
// leaves: the RNG is seeded from rDIV (pokered/engine/math/random.asm), so
// the cycle count is the seed and re-running after any edit rolls a
// different game. Read a dump back with the probe:
//
//	PROBE_STATE=failure/TestGymBoulderBadge.state go test ./skill -run '^TestProbe$' -v
const FailureDir = "failure"

// fixtureVersion is embedded in fixture filenames. Bump it whenever the boot
// sequence or the definition of a valid state changes: a new version
// invalidates every stale fixture at once.
const fixtureVersion = 4

// builders maps a fixture name to the function that produces it from a
// freshly booted emulator.
var builders = map[string]func(*emu.Emu) error{}

// Register associates a fixture name with the function that produces it
// from a freshly booted emulator.
func Register(name string, build func(*emu.Emu) error) {
	builders[name] = build
}

// maxBattles bounds the wild encounters Travel may fight on the way to a
// fixture destination. Measured on the real ROM, Pallet -> Viridian Pokemon
// Center reports exactly one battle (Route 1's Pidgey); 20 is headroom, not
// expectation.
const maxBattles = 20

func init() {
	// reds_bedroom: the state immediately after BootToOverworld.
	Register("reds_bedroom", func(*emu.Emu) error { return nil })

	// post_starter: the state immediately after the opening story: starter
	// taken and the rival battle won.
	Register("post_starter", starter)

	// pallet_town: post-story, walked to Pallet Town. GoTo is safe here:
	// the route never leaves map 0x00 and the town has no grass.
	Register("pallet_town", func(e *emu.Emu) error {
		if err := starter(e); err != nil {
			return err
		}
		dest, err := place("pallet town")
		if err != nil {
			return err
		}
		return skill.GoTo(e, e.ROM(), dest)
	})

	// viridian_city and viridian_pokecenter: post-story, walked across
	// Route 1's tall grass. Travel, not GoTo: GoTo aborts on a wild battle
	// by design, so built with GoTo these fixtures would fail
	// non-deterministically at the first Pidgey.
	Register("viridian_city", func(e *emu.Emu) error {
		return travelTo(e, "viridian city")
	})
	Register("viridian_pokecenter", func(e *emu.Emu) error {
		return travelTo(e, "viridian pokemon center")
	})

	// post_errand: post-starter plus the completed Oak's parcel errand:
	// parcel delivered, Pokedex received, and Viridian City's north gate
	// crossed. Captured after the gate walk, so the fixture ends at (19,8)
	// on map 0x01 — one tile north of the gate line (19,9), south of the
	// Route 2 exit at the map's north edge. Later tasks (S5b-4 onward,
	// e.g. Travel to Pewter) load it instead of replaying the errand.
	Register("post_errand", func(e *emu.Emu) error {
		if err := starter(e); err != nil {
			return err
		}
		romData := e.ROM()
		policy := skill.StatAwareMove(romData)
		if err := skill.OaksParcel(e, romData, policy); err != nil {
			return err
		}
		dest, err := place("viridian city")
		if err != nil {
			return err
		}
		if _, err := skill.Travel(e, romData, dest, policy, maxBattles); err != nil {
			return err
		}
		// (19,10) is the tile directly south of the gate line; the
		// approach from (23,26) stays south of the gate.
		if err := skill.GoTo(e, romData, skill.Destination{Map: 0x01, X: 19, Y: 10}); err != nil {
			return err
		}
		// The crossing itself: two northward steps, ending on (19,8).
		if err := skill.StepOnce(e, world.StepUp); err != nil {
			return fmt.Errorf("fixture post_errand: step (19,10)->(19,9): %w", err)
		}
		if err := skill.StepOnce(e, world.StepUp); err != nil {
			return fmt.Errorf("fixture post_errand: gate step (19,9)->(19,8): %w", err)
		}
		return nil
	})
}

// starter runs the opening story from a freshly booted game. GetStarter is
// idempotent, so calling it first in every builder is correct and cheap.
// Squirtle with StatAwareMove is the measured combination: FirstUsableMove
// loses the rival battle every time, which would leave every fixture
// unbuildable. The ROM bytes come from the emulator itself: emu.Open read
// the file named by POKEMON_RED_ROM once, so e.ROM() reuses them rather than
// re-reading the file per fixture.
func starter(e *emu.Emu) error {
	romData := e.ROM()
	return skill.GetStarter(e, romData, skill.StarterSquirtle, skill.StatAwareMove(romData))
}

// travelTo runs the story, then Travel to the named place, fighting the wild
// encounters along the way.
func travelTo(e *emu.Emu, name string) error {
	if err := starter(e); err != nil {
		return err
	}
	dest, err := place(name)
	if err != nil {
		return err
	}
	_, err = skill.Travel(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), maxBattles)
	return err
}

// place resolves a fixture destination from skill.Place, the single correct
// source of coordinates.
func place(name string) (skill.Destination, error) {
	d, ok := skill.Place(name)
	if !ok {
		return skill.Destination{}, fmt.Errorf("fixture: Place: %s not found", name)
	}
	return d, nil
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
			dumpOnFailure(t, e)
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
	dumpOnFailure(t, e)
	return e
}

// dumpOnFailure closes e when the test ends, and first writes its save state
// to FailureDir if the test failed. A journey test that loses a battle and a
// journey test that hits a real bug fail the same way from the outside; the
// state says which, and it cannot be recovered by re-running, because any
// edit to a frame budget upstream reseeds every roll after it.
func dumpOnFailure(t *testing.T, e *emu.Emu) {
	t.Helper()
	t.Cleanup(func() {
		defer e.Close()
		if !t.Failed() {
			return
		}
		b, err := e.SaveState()
		if err != nil {
			t.Logf("failure dump: SaveState: %v", err)
			return
		}
		if err := os.MkdirAll(FailureDir, 0o755); err != nil {
			t.Logf("failure dump: mkdir %s: %v", FailureDir, err)
			return
		}
		path := filepath.Join(FailureDir, strings.ReplaceAll(t.Name(), "/", "_")+".state")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Logf("failure dump: write %s: %v", path, err)
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		t.Logf("failure dump: wrote %s (read it with PROBE_STATE=%s)", path, abs)
	})
}
