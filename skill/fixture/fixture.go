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
	"errors"
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
const fixtureVersion = 5

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

// maxBlackouts bounds how many times a fixture journey may black out and
// retry. A blackout is a recoverable interruption for a fixture, not a
// failure: the game fully heals the party (ResetStatusAndHalveMoneyOnBlackout
// ends in HealParty) and respawns the player on the last town's fly-warp
// spot (a Route 1 blackout lands on Pallet Town (5,6); there is no center
// there), so a fresh attempt starts from a healthy party. A journey that
// blackouts this many times is a real problem the build should report, not
// loop on.
const maxBlackouts = 3

// Travel walks to dest like skill.Travel, but treats a blackout as a
// recoverable interruption: skill.Travel returns the typed ErrBlackedOut
// instead of walking on (S6-0d), and the caller's decision here is to
// re-attempt from the respawn spot, where the party is fully healed. The
// result is the one that ended the journey; the error is nil only when the
// journey actually arrived.
func Travel(e *emu.Emu, dest skill.Destination, policy skill.MovePolicy, maxBattles int) (skill.TravelResult, error) {
	for blackouts := 0; ; blackouts++ {
		res, err := skill.Travel(e, e.ROM(), dest, policy, maxBattles)
		if err == nil || !errors.Is(err, skill.ErrBlackedOut) || blackouts >= maxBlackouts {
			return res, err
		}
	}
}

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

	// viridian_mart: post-errand, walked to the Viridian Mart's approach tile
	// (Place("viridian mart")). It is built on the completed Oak's-parcel
	// errand, not a bare journey, because the clerk only opens the shop once
	// EVENT_OAK_GOT_PARCEL is set (pokered/scripts/ViridianMart.asm,
	// ViridianMartCheckParcelDeliveredScript): before that, talking to the
	// clerk hands over the parcel instead of selling. The player has earned
	// money from the wild battles on the way in, so a small purchase is
	// affordable; the test reads the actual amount rather than assuming it.
	Register("viridian_mart", func(e *emu.Emu) error {
		if err := starter(e); err != nil {
			return err
		}
		romData := e.ROM()
		policy := skill.StatAwareMove(romData)
		if err := skill.OaksParcel(e, romData, policy); err != nil {
			return err
		}
		dest, err := place("viridian mart")
		if err != nil {
			return err
		}
		_, err = Travel(e, dest, policy, maxBattles)
		return err
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
		if _, err := Travel(e, dest, policy, maxBattles); err != nil {
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

	// post_pokeballs: post_errand state plus the five POKE_BALLs Oak gives
	// after the Route 22 rival battle: EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE
	// and EVENT_GOT_POKEBALLS_FROM_OAK set, 5x POKE_BALL in the bag. Ends
	// in Oak's lab, controllable. Catching tasks (S6-3 onward) load this
	// instead of replaying starter + errand + Route 22 battle from boot in
	// every run.
	Register("post_pokeballs", func(e *emu.Emu) error {
		if err := starter(e); err != nil {
			return err
		}
		romData := e.ROM()
		policy := skill.StatAwareMove(romData)
		if err := skill.OaksParcel(e, romData, policy); err != nil {
			return err
		}
		// The post-parcel lead (level 7) loses the Route 22 rival battle —
		// his party is Pidgey Lv9 + Bulbasaur Lv8 — so train on Route 1's
		// grass first; skill.GetPokeBalls itself does not train.
		if err := trainForRoute22Rival(e, romData, policy); err != nil {
			return err
		}
		return skill.GetPokeBalls(e, romData, policy)
	})
}

// trainForRoute22Rival levels the lead to skill.Route22RivalLeadLevel on
// Route 1's grass, healing at the Viridian center before every chunk of two
// levels. The lead takes cumulative damage across battles — measured
// 2026-08-28: a fully healed level-7 lead won eight straight fights and
// lost the ninth to a level-3 Pidgey on the damage it had accumulated —
// and with a one-mon party that loss is a blackout. Two-level chunks keep
// the damage between heals survivable; a blackout mid-chunk is still
// recoverable, because the respawn fully heals and the next chunk travels
// back.
func trainForRoute22Rival(e *emu.Emu, romData []byte, policy skill.MovePolicy) error {
	center, err := place("viridian pokemon center")
	if err != nil {
		return err
	}
	route1, err := place("route 1")
	if err != nil {
		return err
	}
	for {
		var mem state.Mem
		state.Snapshot(e, &mem)
		lead := int(state.DecodeParty(&mem).Mons[0].Level)
		if lead >= skill.Route22RivalLeadLevel {
			return nil
		}
		if _, err := Travel(e, center, policy, maxBattles); err != nil {
			return fmt.Errorf("fixture post_pokeballs: travel to the center: %w", err)
		}
		if err := skill.Heal(e); err != nil {
			return fmt.Errorf("fixture post_pokeballs: Heal: %w", err)
		}
		if _, err := Travel(e, route1, policy, maxBattles); err != nil {
			return fmt.Errorf("fixture post_pokeballs: travel to route 1: %w", err)
		}
		res, err := skill.Train(e, romData, lead+2, policy, 12)
		if err != nil {
			return fmt.Errorf("fixture post_pokeballs: Train: %w (start=%d end=%d battles=%d reached=%v blackedOut=%v)",
				err, res.StartLevel, res.EndLevel, res.Battles, res.Reached, res.BlackedOut)
		}
	}
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
	_, err = Travel(e, dest, skill.StatAwareMove(e.ROM()), maxBattles)
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

// LoadState returns an emulator restored to the named fixture, generating
// and caching the fixture first if it does not exist. It is the plain,
// non-test core of the named save-state layer: cached to disk, validated on
// write AND on read, versioned by fixtureVersion — so code outside a test
// (a replay of a failed run) can stand on the same validated states the
// tests do. The POKEMON_RED_ROM environment variable must name a ROM.
//
// A cached fixture is validated before being trusted: if it fails validation
// (state.Controllable is false) the file is deleted and the fixture is
// regenerated from scratch, so a poisoned cache entry can never be returned.
func LoadState(name string) (*emu.Emu, error) {
	rom := os.Getenv("POKEMON_RED_ROM")
	if rom == "" {
		return nil, fmt.Errorf("fixture %s: POKEMON_RED_ROM not set; cannot generate or load", name)
	}

	path := fixturePath(name)
	if b, err := os.ReadFile(path); err == nil {
		e, err := emu.Open(rom)
		if err != nil {
			return nil, fmt.Errorf("fixture %s: emu.Open: %w", name, err)
		}
		if err := e.LoadState(b); err != nil {
			e.Close()
			return nil, fmt.Errorf("fixture %s: LoadState: %w", name, err)
		}
		if _, ok := validateState(e); ok {
			return e, nil
		}
		e.Close()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("fixture %s: remove poisoned %s: %w", name, path, err)
		}
		// The cached state failed validation; it is deleted and regenerated
		// below.
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("fixture %s: read %s: %w", name, path, err)
	}

	build, ok := builders[name]
	if !ok {
		return nil, fmt.Errorf("fixture %s: not registered", name)
	}
	e, err := emu.Open(rom)
	if err != nil {
		return nil, fmt.Errorf("fixture %s: emu.Open: %w", name, err)
	}
	if _, err := skill.BootToOverworld(e); err != nil {
		e.Close()
		return nil, fmt.Errorf("fixture %s: boot: %w", name, err)
	}
	if err := build(e); err != nil {
		e.Close()
		return nil, fmt.Errorf("fixture %s: build: %w", name, err)
	}
	gs, ok := validateState(e)
	if !ok {
		e.Close()
		return nil, fmt.Errorf("fixture %s: generated state failed validation: MapID=%#04x X=%d Y=%d CurMapWidth=%d CurMapHeight=%d",
			name, gs.Player.MapID, gs.Player.X, gs.Player.Y, gs.World.Width, gs.World.Height)
	}
	b, err := e.SaveState()
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("fixture %s: SaveState: %w", name, err)
	}
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		e.Close()
		return nil, fmt.Errorf("fixture %s: mkdir %s: %w", name, Dir, err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		e.Close()
		return nil, fmt.Errorf("fixture %s: write %s: %w", name, path, err)
	}
	return e, nil
}

// Load is the *testing.T wrapper over LoadState for the existing callers:
// it skips when POKEMON_RED_ROM is unset and fails the test (with the usual
// failure dump) on any error.
func Load(t *testing.T, name string) *emu.Emu {
	t.Helper()
	if os.Getenv("POKEMON_RED_ROM") == "" {
		t.Skip("POKEMON_RED_ROM not set; cannot generate or load fixture " + name)
	}
	e, err := LoadState(name)
	if err != nil {
		t.Fatalf("%v", err)
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
