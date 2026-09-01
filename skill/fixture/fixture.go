// Package fixture caches expensive emulator boot sequences as save states so
// tests start from a deterministic, millisecond-fast state instead of replay
// ing thousands of frames.
//
// Fixtures are derived from a commercial ROM, so they are generated on demand
// from the ROM named by POKEMON_RED_ROM and cached under ResolveDir();
// they are never
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

// DefaultDir is the in-repo fixture cache, used when nothing overrides it.
// It is gitignored (.gitignore: *.state and testdata/fixtures/).
const DefaultDir = "testdata/fixtures"

// ResolveDir is where generated fixtures are cached. POKEPILOT_FIXTURE_DIR
// overrides it, so a clean worktree can share a cache instead of rebuilding
// post_pokeballs from post_starter on every run.
func ResolveDir() string {
	if d := os.Getenv("POKEPILOT_FIXTURE_DIR"); d != "" {
		return d
	}
	return DefaultDir
}

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

	// route1: post-story, walked to Route 1's center (Place("route 1"),
	// (5,14) on map 0x0c). The state a talk objective on the route stands
	// beside: the NPC at (5,24) is reached only through tall grass, so
	// TalkAt's approach crosses wild encounters. Built with Travel, not
	// GoTo — see viridian_city.
	Register("route1", func(e *emu.Emu) error {
		return travelTo(e, "route 1")
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
	Register("post_errand", postErrand)

	// forest_north_gate: the full road to the forest's NORTH gate (map
	// 0x2f, (5,1)): post_errand, up Route 2's south band to the south
	// gate, across the forest, up to the north gate — with the lead ground
	// to forestGrindLevel in the forest itself, the same
	// session/heal/blackout grind the gym journey used to run at test
	// time. Two consumers: the gym tests start here instead of re-walking
	// the road and re-grinding on every run, and the gate's standing
	// Super Nerd (home tile (3,2),
	// pokered/data/maps/objects/ViridianForestNorthGate.asm) is a two-step
	// walk and one Talk away — the requirement-harvest test (S10-9) needs
	// a state where its line is reachable, and this is it.
	Register("forest_north_gate", forestNorthGate)

	// pewter_city: forest_north_gate plus the last road leg: the gate
	// crossing, up Route 2's north band into Pewter City, ending at
	// Place("pewter city") (14,8).
	Register("pewter_city", pewterCity)

	// post_boulder: pewter_city plus the Brock fight: healed at the Pewter
	// Center, beaten at the gym (ResultWon required — a fixture built on a
	// lost fight is worse than no fixture), the Boulder Badge bit set in
	// RAM, healed again, and back at Place("pewter city"). The Cascade
	// test starts here instead of re-running the whole Pewter half, and
	// Pewter's east exit — locked until EVENT_BEAT_BROCK — is open.
	Register("post_boulder", postBoulder)

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

// postErrand runs the story and the Oak's parcel errand, then crosses
// Viridian City's north gate. It is a named function because the fixtures
// further along the road (forest_north_gate and its descendants) build on
// it from a freshly booted emulator, the way pallet_town builds on
// starter.
func postErrand(e *emu.Emu) error {
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
}

// forestNorthGate walks the road to the forest's north gate and grounds the
// lead in the forest on the way. The legs are the proven
// TestGymBoulderBadge scaffold: the south gate (0x32) is the only forest
// crossing from Route 2's south band, and the safe spot (17,40) is clear of
// every warp on the forest map, so the grind ping-pong cannot step on the
// south warp pocket (15,47)-(18,47) and get carried back to Route 2's
// dead-end south band.
func forestNorthGate(e *emu.Emu) error {
	if err := postErrand(e); err != nil {
		return err
	}
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	// Leg 1: Viridian City to the south gate, one row below the (5,0)
	// forest warp; the gate is entered from Route 2's warp (3,43).
	if _, err := Travel(e, skill.Destination{Map: 0x32, X: 5, Y: 1}, policy, maxBattles); err != nil {
		return fmt.Errorf("fixture forest_north_gate: travel to the south gate: %w", err)
	}
	// Leg 2: the south gate into the forest at Place("viridian forest")
	// (17,43).
	forest, err := place("viridian forest")
	if err != nil {
		return err
	}
	if _, err := Travel(e, forest, policy, maxBattles); err != nil {
		return fmt.Errorf("fixture forest_north_gate: travel to the forest: %w", err)
	}
	// The gate drops the player in the south warp pocket; walk up into the
	// open forest before any grind.
	safeSpot := skill.Destination{Map: 0x33, X: 17, Y: 40}
	if _, err := Travel(e, safeSpot, policy, maxBattles); err != nil {
		return fmt.Errorf("fixture forest_north_gate: travel to the safe spot: %w", err)
	}
	if err := trainInForest(e, romData, policy, safeSpot); err != nil {
		return err
	}
	// Leave the forest healthy: the grind ends the moment the level is
	// reached, which can be with a statused or hurt lead, and a lead like
	// that blacks out on the gate leg. The detour is taken only when the
	// lead is actually in danger; a healthy lead walks straight to the
	// gate.
	var mem state.Mem
	state.Snapshot(e, &mem)
	lead := state.DecodeParty(&mem).Mons[0]
	if lead.Status != 0 || int(lead.HP)*2 < int(lead.MaxHP) {
		center, err := place("viridian pokemon center")
		if err != nil {
			return err
		}
		if _, err := Travel(e, center, policy, 5); err != nil {
			if !errors.Is(err, skill.ErrBlackedOut) {
				return fmt.Errorf("fixture forest_north_gate: travel to the center before the gate: %w", err)
			}
			if err := settleBlackout(e); err != nil {
				return err
			}
		} else if err := skill.Heal(e); err != nil {
			return fmt.Errorf("fixture forest_north_gate: heal before the gate: %w", err)
		}
	}
	// Leg 3: the forest to the north gate.
	if _, err := Travel(e, skill.Destination{Map: 0x2F, X: 5, Y: 1}, policy, maxBattles); err != nil {
		return fmt.Errorf("fixture forest_north_gate: travel to the north gate: %w", err)
	}
	return nil
}

// pewterCity is forest_north_gate plus the last road leg: the gate crossing
// is an ordinary leg (Traverse picks the reachable (5,0) warp tile itself),
// then up Route 2's north band into Pewter City.
func pewterCity(e *emu.Emu) error {
	if err := forestNorthGate(e); err != nil {
		return err
	}
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)
	dest, err := place("pewter city")
	if err != nil {
		return err
	}
	if _, err := Travel(e, dest, policy, maxBattles); err != nil {
		return fmt.Errorf("fixture pewter_city: travel to Pewter City: %w", err)
	}
	return nil
}

// postBoulder is pewter_city plus the Brock fight. The fight must start
// with the lead at full HP and no status — the same positive precondition
// the gym test enforces — and it must be WON: a loss is the game
// answering, and a fixture that depends on a lost fight is worse than no
// fixture, so the build fails rather than caching it. The build ends back
// at Place("pewter city") with the party healed.
func postBoulder(e *emu.Emu) error {
	if err := pewterCity(e); err != nil {
		return err
	}
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)
	gym, err := place("pewter gym")
	if err != nil {
		return err
	}
	center, err := place("pewter pokemon center")
	if err != nil {
		return err
	}
	if _, err := Travel(e, center, policy, 5); err != nil {
		return fmt.Errorf("fixture post_boulder: travel to the Pewter Center: %w", err)
	}
	if err := skill.Heal(e); err != nil {
		return fmt.Errorf("fixture post_boulder: heal: %w", err)
	}
	if _, err := Travel(e, gym, policy, 5); err != nil {
		return fmt.Errorf("fixture post_boulder: travel to the gym: %w", err)
	}
	outcome, err := skill.Gym(e, romData, policy)
	if err != nil {
		return fmt.Errorf("fixture post_boulder: Gym: %w", err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("fixture post_boulder: the gym fight ended %d, want a win; the fixture is not buildable this run", outcome)
	}
	// The badge is written by the game during the fight; the poll is the
	// same trip wire the gym test runs.
	for i := 0; i < 3000; i++ {
		e.StepFrame()
		var mem state.Mem
		state.Snapshot(e, &mem)
		if state.DecodeProgress(&mem).Has(state.BadgeBoulder) {
			break
		}
	}
	if _, err := Travel(e, center, policy, 5); err != nil {
		return fmt.Errorf("fixture post_boulder: travel to the Pewter Center after the gym: %w", err)
	}
	if err := skill.Heal(e); err != nil {
		return fmt.Errorf("fixture post_boulder: heal after the gym: %w", err)
	}
	city, err := place("pewter city")
	if err != nil {
		return err
	}
	if _, err := Travel(e, city, policy, maxBattles); err != nil {
		return fmt.Errorf("fixture post_boulder: travel back to Pewter City: %w", err)
	}
	return nil
}

// The forest grind the forest_north_gate fixture runs, in the same shape as
// the gym journey's runtime grind (skill/gym_test.go): a water lead at 12
// beats Brock's rock team and the forest leg's forced rival battle, so the
// target is 12; a few battles per session, because Train has no HP
// awareness and a one-mon party that grinds hard blacks out.
const (
	forestGrindLevel        = 12
	forestTrainBattleBudget = 4
	forestMaxHealDetours    = 6
	forestMaxPhaseRetries   = 3
	phaseShiftFrames        = 123 // the frame shift that breaks a no-encounter phase
)

// trainInForest grounds the lead to forestGrindLevel on the forest's grass,
// the same session/heal/blackout structure the gym journey used to run at
// test time: one session is a few battles, a hurt lead takes a detour to
// the Viridian Center, a blackout is recovery (the respawn fully heals),
// and a no-encounter phase is broken with a frame shift — the retry the
// journey tests use, because the encounter roll mixes in the cycle count
// and a blackout's fade frames can shift the grind into a dry cycle.
func trainInForest(e *emu.Emu, romData []byte, policy skill.MovePolicy, safeSpot skill.Destination) error {
	center, err := place("viridian pokemon center")
	if err != nil {
		return err
	}
	detours, phaseRetries := 0, 0
	for {
		var mem state.Mem
		state.Snapshot(e, &mem)
		lead := state.DecodeParty(&mem).Mons[0]
		if int(lead.Level) >= forestGrindLevel {
			return nil
		}
		res, err := skill.Train(e, romData, forestGrindLevel, policy, forestTrainBattleBudget)
		if err != nil {
			if strings.Contains(err.Error(), "no-encounter phase") && phaseRetries < forestMaxPhaseRetries {
				phaseRetries++
				e.StepFrames(phaseShiftFrames)
				continue
			}
			return fmt.Errorf("fixture forest_north_gate: Train: %w (start=%d end=%d battles=%d reached=%v blackedOut=%v)",
				err, res.StartLevel, res.EndLevel, res.Battles, res.Reached, res.BlackedOut)
		}
		state.Snapshot(e, &mem)
		lead = state.DecodeParty(&mem).Mons[0]
		if res.BlackedOut {
			if err := settleBlackout(e); err != nil {
				return err
			}
			if _, err := Travel(e, safeSpot, policy, maxBattles); err != nil {
				return fmt.Errorf("fixture forest_north_gate: travel back to the forest after a blackout: %w", err)
			}
			continue
		}
		if int(lead.Level) < forestGrindLevel && (res.Retreated || int(lead.HP)*3 < int(lead.MaxHP) || lead.Status != 0) {
			detours++
			if detours > forestMaxHealDetours {
				return fmt.Errorf("fixture forest_north_gate: the lead is level %d after %d heal detours and cannot reach %d",
					lead.Level, detours, forestGrindLevel)
			}
			if _, err := Travel(e, center, policy, 5); err != nil {
				if !errors.Is(err, skill.ErrBlackedOut) {
					return fmt.Errorf("fixture forest_north_gate: travel to the center to heal: %w", err)
				}
				if err := settleBlackout(e); err != nil {
					return err
				}
			} else if err := skill.Heal(e); err != nil {
				return fmt.Errorf("fixture forest_north_gate: heal: %w", err)
			}
			if _, err := Travel(e, safeSpot, policy, maxBattles); err != nil {
				return fmt.Errorf("fixture forest_north_gate: travel back to the forest after healing: %w", err)
			}
		}
	}
}

// settleBlackout steps frames without input until a blackout's respawn warp
// has landed: the party fully healed and the player controllable. Travel
// returns ErrBlackedOut before the respawn completes, and walking in that
// gap replays stale steps from the wrong map (the test helper of the same
// name in skill/gym_test.go holds the measurement).
func settleBlackout(e *emu.Emu) error {
	for i := 0; i < 200; i++ {
		e.StepFrames(25)
		var mem state.Mem
		state.Snapshot(e, &mem)
		if !state.Controllable(&mem) {
			continue
		}
		lead := state.DecodeParty(&mem).Mons[0]
		if int(lead.HP) == int(lead.MaxHP) && lead.Status == 0 {
			return nil
		}
	}
	return fmt.Errorf("fixture: settleBlackout: the respawn warp did not land within 5000 frames")
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
	return filepath.Join(ResolveDir(), fmt.Sprintf("%s.v%d.state", name, fixtureVersion))
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
	if err := os.MkdirAll(ResolveDir(), 0o755); err != nil {
		e.Close()
		return nil, fmt.Errorf("fixture %s: mkdir %s: %w", name, ResolveDir(), err)
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
