package skill_test

import (
	"testing"
	"time"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// speciesCaterpie is the species byte wEnemyMonSpecies holds for CATERPIE.
// It is the ROM's pokemon index, not a dex number: in this decompilation
// CATERPIE = $7B (pokered/constants/pokemon_constants.asm), and a live hunt
// measured WEEDLE=$70, KAKUNA=$71, METAPOD=$7C against the same table.
const speciesCaterpie uint8 = 0x7B

// catchAttempts bounds the test's independent tries. Each try reloads the
// fixture (a fresh five-ball bag and party) and hunts from scratch; a try
// that misses every Caterpie or burns all five balls is part of the game,
// not a bug, so it is logged and retried. Measured against the ROM's catch
// formula (pokered/engine/items/item_effects.asm), a full-HP Lv3 Caterpie
// takes 79/256 ≈ 31% per ball, so five balls all miss with ~1.2% probability
// per wanted encounter — one try is not evidence either way, several are.
const catchAttempts = 5

// TestCatchCaterpie is S6-3: from the post_pokeballs fixture (five POKE
// BALLs in the bag), travel to Viridian Forest and Catch a Caterpie. The
// positive postcondition is that state.DecodeParty reports ONE MORE member
// than before, of the wanted species — not that a ball was consumed or a
// battle ended.
//
// It is a full journey (Travel across Route 1's grass plus a stochastic
// hunt for a species that is 1 of Viridian Forest's 10 encounter entries),
// so it is guarded out of -short the way the S6-0b journey tests are and
// proven separately:
//
//	POKEMON_RED_ROM=roms/pokemon_red.gb go test ./skill -run TestCatchCaterpie -v
func TestCatchCaterpie(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey (Viridian Forest hunt); run without -short, see TestCatchCaterpie docs")
	}

	var last skill.CatchResult
	for attempt := 1; attempt <= catchAttempts; attempt++ {
		m := fixture.Load(t, "post_pokeballs")
		// A fixture load restores the emulator byte for byte, and the
		// encounter and catch rolls are seeded from the frame counter, so
		// without a phase shift every attempt replays one identical run and
		// the retry loop is dead weight. MEASURED 2026-08-29: attempts 1 and
		// 2 of an unshifted run both ended outcome=2 balls=5 encounters=16,
		// the same numbers to the digit. Stepping a distinct number of
		// frames per attempt (no input, so the world does not move) re-phases
		// the rolls and makes the attempts genuinely independent.
		m.StepFrames((attempt - 1) * 97)
		romData := m.ROM()
		policy := skill.StatAwareMove(romData)

		// Preconditions: the fixture carries exactly five balls and a
		// known party size, so the assertions measure Catch itself.
		var mem state.Mem
		state.Snapshot(m, &mem)
		if q := bagQty(t, m, skill.ItemPokeBall); q != 5 {
			t.Fatalf("fixture precondition: expected five POKE BALLS in the bag, got %d", q)
		}
		partyBefore := state.DecodeParty(&mem)
		if partyBefore.Count < 1 || partyBefore.Count >= 6 {
			t.Fatalf("fixture precondition: unexpected party size %d", partyBefore.Count)
		}

		forest, ok := skill.Place("viridian forest")
		if !ok {
			t.Fatal(`Place "viridian forest" not found`)
		}
		if _, err := skill.Travel(m, romData, forest, policy, 20); err != nil {
			t.Fatalf("attempt %d: travel to Viridian Forest: %v", attempt, err)
		}

		start := time.Now()
		res, err := skill.Catch(m, romData, []uint8{speciesCaterpie}, policy, 5)
		last = res
		t.Logf("attempt %d took %v: outcome=%d species=%#02x balls=%d encounters=%d",
			attempt, time.Since(start), res.Outcome, res.Species, res.BallsThrown, res.Encounters)
		if err != nil {
			t.Fatalf("attempt %d: Catch: %v (result %+v)", attempt, err, res)
		}

		if res.Outcome == skill.OutcomeCaught {
			// The typed outcome must agree with the RAM postcondition.
			state.Snapshot(m, &mem)
			partyAfter := state.DecodeParty(&mem)
			if partyAfter.Count != partyBefore.Count+1 {
				t.Fatalf("postcondition: party has %d members, want %d (one more than before)",
					partyAfter.Count, partyBefore.Count+1)
			}
			newMon := partyAfter.Mons[partyAfter.Count-1]
			if newMon.Species != speciesCaterpie {
				t.Fatalf("postcondition: new member is species %#02x, want CATERPIE (%#02x): %+v",
					newMon.Species, speciesCaterpie, newMon)
			}
			if q := bagQty(t, m, skill.ItemPokeBall); q != 5-res.BallsThrown {
				t.Fatalf("postcondition: bag holds %d POKE BALLs, want %d (5 minus %d thrown)",
					q, 5-res.BallsThrown, res.BallsThrown)
			}
			if !state.Controllable(&mem) {
				t.Fatalf("postcondition: player not controllable: %+v", state.DecodePlayer(&mem))
			}
			t.Logf("caught CATERPIE lv%d in %d ball(s) after %d encounter(s); party now %d members",
				newMon.Level, res.BallsThrown, res.Encounters, partyAfter.Count)
			return
		}

		// A miss is a game outcome, not a failure: log it and try again
		// from a fresh fixture state.
		t.Logf("attempt %d did not catch (outcome=%d balls=%d encounters=%d); retrying",
			attempt, res.Outcome, res.BallsThrown, res.Encounters)
	}
	t.Fatalf("no catch in %d attempts; last result %+v — if this is the all-balls-missed case it is the measured ~1.2%% per-encounter miss compounding over a sparse hunt; read the failure state before changing anything",
		catchAttempts, last)
}
