package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// CatchOutcome is how a Catch call ended. The outcomes that are part of the
// game — the ball broke and the Pokemon fled, the balls ran out, the target
// fainted — are values here, not errors: the caller decides what each means
// for its plan. Only setup failures (no grass, a stuck battle, a blackout)
// come back as errors.
type CatchOutcome int

const (
	// OutcomeCaught: the party grew by one member of a wanted species.
	OutcomeCaught CatchOutcome = iota
	// OutcomeFled: every thrown ball broke and the Pokemon ran away.
	OutcomeFled
	// OutcomeOutOfBalls: maxBalls were thrown (or the bag ran dry) without
	// a catch; the battle was then ended by fighting.
	OutcomeOutOfBalls
	// OutcomeTargetFainted: the target's HP reached 0 before it was caught.
	// Catch never attacks a wanted target, so this should be unreachable;
	// it is reported rather than swallowed if it ever happens.
	OutcomeTargetFainted
)

// CatchResult reports what a Catch call did.
type CatchResult struct {
	Outcome     CatchOutcome
	Species     uint8 // the caught species (OutcomeCaught only), else 0
	BallsThrown int   // balls consumed on the wanted target
	Encounters  int   // wild battles met while hunting, fought or caught
}

// Bounds on the hunt.
//
// CORRECTED 2026-08-29. An earlier version of this comment said CATERPIE is
// "1 of 10 entries, so ~10% of encounters, and (9/10)^32 ≈ 3.4% all-miss".
// That is WRONG: the ten slots are not equiprobable. CATERPIE sits in slot 7
// of Viridian Forest's RED table (data/wild/maps/ViridianForest.asm: Weedle,
// Kakuna, Weedle, Weedle, Kakuna, Kakuna, Metapod, CATERPIE, then Pikachu,
// Pikachu), and WildMonEncounterSlotChances (data/wild/probabilities.asm)
// gives slot 7 13/256 = 5.1%. So a Caterpie is expected once every ~20
// encounters, and the all-miss probability at catchHuntCap is
// (1-0.051)^32 ≈ 19%, not 3.4%.
//
// MEASURED over five phase-shifted hunts from post_pokeballs (2026-08-29):
// two met a Caterpie and caught it, one met one and lost it to five broken
// balls, and TWO hit the caps without ever meeting one (32 encounters / 454
// legs, and 26 encounters / 500 legs). That 2-in-5 matches the corrected
// 19% far better than the old 3.4%.
//
// The caps are deliberately LEFT AS THEY ARE: a hunt that misses is a
// reported outcome, and widening a budget to make a stochastic search
// succeed more often is what this plan bans. The number is written down
// here and in RUNNOTES.md so S6-4 and S6-12 can decide with it. The leg
// bound sizes the other axis: the forest's grass rolls at 8/256 per step
// (def_grass_wildmons 8), and measured hunts take on the order of 15 legs
// per encounter, so 500 legs is the same order as the encounter cap — and
// it does bind first sometimes, as the 26-encounter sample above shows.
const (
	catchHuntCap   = 32  // wanted-species encounters met before giving up
	catchGrassLegs = 500 // total grass legs the hunt may spend on encounters

	// battleEndSettle bounds the wait for a just-ended battle to clear RAM
	// and the player to become controllable again.
	battleEndSettle = 5000

	// throwPollFrames is how far the throw-result loop steps between RAM
	// reads. Small enough that the nickname prompt is answered promptly,
	// large enough that the loop is not a per-frame snapshot.
	throwPollFrames = 10
)

// ErrCatchBlackout reports that a non-wanted battle ended in a loss: the
// party blacked out and the hunt cannot continue from where it stands.
var ErrCatchBlackout = errors.New("skill: Catch: blacked out while fighting a non-wanted battle")

// Catch hunts the tall grass on the current map until it meets a wild
// Pokemon of one of the species in want, then throws POKE BALLs at it (via
// S6-2's UseItem) until it is caught or maxBalls are spent.
//
// An encounter that is not wanted is fought normally with policy and the
// hunt continues; Catch never reuses StatAwareMove against a wanted target,
// because a policy that always attacks would faint the thing being caught.
// Weakening the target first is deliberately out of scope: a full-HP
// Caterpie (catch rate 255) passes the formula's first check (rand(256) <
// CatchRate) with probability 255/256, and at full HP its second check rolls
// against ((MaxHP*255)/12)/(MaxHP/4) = 255*4/12 = 85, so each ball is
// 85/256 ≈ 33%.
//
// CORRECTED 2026-08-29: an earlier version of this comment put five-ball
// failure at "about 1.2%". The arithmetic is (1-0.33)^5 ≈ 13%, an order of
// magnitude worse, and MEASURED agrees — of three wanted encounters across
// five phase-shifted hunts, one burned all five balls without a catch
// (balls per catch on the other two: 3 and 1). FIVE BALLS ARE NOT ENOUGH to
// make a catch reliable; S6-6 should size the mart trip from ~3 balls per
// catch plus a ~13% chance of losing all five on a given target.
//
// The postcondition for OutcomeCaught is positive: state.DecodeParty
// reports one MORE member than before, and that member's species is in
// want. A dropped bag count or an ended battle is not a catch.
//
// Both axes are bounded: at most maxBalls throws, and at most catchHuntCap
// encounters met while hunting.
func Catch(m *emu.Emu, romData []byte, want []uint8, policy MovePolicy, maxBalls int) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: Catch: nil policy")
	}
	if len(want) == 0 {
		return CatchResult{}, fmt.Errorf("skill: Catch: want is empty")
	}
	if maxBalls <= 0 {
		return CatchResult{}, fmt.Errorf("skill: Catch: maxBalls must be > 0, got %d", maxBalls)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return CatchResult{}, fmt.Errorf("skill: Catch: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	before := int(state.DecodeParty(&mem).Count)
	res := CatchResult{}

	// The hunt ping-pongs the player between two nearby grass cells itself
	// (the same pair EnterWildBattle picks), because a dry attempt — no
	// encounter rolled in a burst of legs — is part of the hunt, not a
	// failure, and only the total leg budget should end it.
	now := currentWorld(m)
	grass, grid, err := grassCells(romData, now.Map)
	if err != nil {
		return res, err
	}
	if len(grass) == 0 {
		return res, fmt.Errorf("skill: Catch: no walkable tall grass on map %#04x", now.Map)
	}
	a, b, ok := grindPair(grass, grid, int(now.X), int(now.Y))
	if !ok {
		return res, fmt.Errorf("skill: Catch: map %#04x has no two walkable grass cells close enough to hunt between", now.Map)
	}

	next := b
	legsSpent := 0
	for res.Encounters < catchHuntCap && legsSpent < catchGrassLegs {
		// One leg: walk to the other grass cell. Stepping onto a fresh grass
		// cell re-rolls the encounter, whether or not one fires on this leg.
		d := Destination{Map: now.Map, X: uint8(next.x), Y: uint8(next.y)}
		if err := GoTo(m, romData, d); err != nil && !errors.Is(err, ErrBattle) {
			return res, fmt.Errorf("skill: Catch: hunt leg %d: %w", legsSpent+1, err)
		}
		legsSpent++
		next = flip(a, b, next)
		if !waitBattleStart(m, 1000) {
			continue
		}
		state.Snapshot(m, &mem)
		bs := state.DecodeBattle(&mem)
		if bs == nil {
			return res, fmt.Errorf("skill: Catch: hunt leg %d reported an encounter but no battle is in progress on map %#04x", legsSpent, m.Peek8(sym.CurMap))
		}
		res.Encounters++

		if !speciesIn(bs.EnemySpecies, want) {
			outcome, err := Battle(m, policy)
			if err != nil {
				return res, fmt.Errorf("skill: Catch: non-wanted battle %d (species %d): %w", res.Encounters, bs.EnemySpecies, err)
			}
			if outcome == state.ResultLost {
				return res, ErrCatchBlackout
			}
			continue
		}

		return catchWanted(m, &mem, want, policy, before, res, maxBalls)
	}
	return res, fmt.Errorf("skill: Catch: %d grass legs and %d encounters without a wanted species (map %#04x)",
		legsSpent, res.Encounters, m.Peek8(sym.CurMap))
}

// catchWanted throws balls at the wanted target in progress and reports the
// outcome. It never attacks: the only way the target takes damage here is a
// bug, which OutcomeTargetFainted exists to name.
func catchWanted(m *emu.Emu, mem *state.Mem, want []uint8, policy MovePolicy, partyBefore int, res CatchResult, maxBalls int) (CatchResult, error) {
	targetFainted := false
	for res.BallsThrown < maxBalls && battleInFlight(m) {
		if err := UseItem(m, ItemPokeBall); err != nil {
			if errors.Is(err, ErrNotInBag) {
				break // the bag is dry before maxBalls: same ending as running out
			}
			return res, fmt.Errorf("skill: Catch: throw %d: %w", res.BallsThrown+1, err)
		}
		res.BallsThrown++

		state.Snapshot(m, mem)
		if bs := state.DecodeBattle(mem); bs != nil && bs.EnemyHP == 0 {
			targetFainted = true
		}

		// Wait for this throw's result: a catch (or the target ending the
		// battle some other way) ends the battle, while a broken ball
		// returns to the FIGHT/ITEM/PKMN/RUN menu. UseItem may return as
		// early as the "used POKE BALL!" text, so the outcome is not yet
		// in RAM when it does.
		ended, err := waitThrowResult(m)
		if err != nil {
			return res, fmt.Errorf("skill: Catch: throw %d: %w", res.BallsThrown, err)
		}
		if !ended {
			continue // the ball broke: throw again
		}
		break // the battle ended: classify below
	}

	if battleInFlight(m) {
		// maxBalls spent (or the bag dry) with the target still in battle:
		// end it by fighting so nothing is left mid-battle. The target was
		// never caught, so using policy now is not the "kill what you are
		// catching" case.
		if _, err := Battle(m, policy); err != nil {
			return res, fmt.Errorf("skill: Catch: ending the uncaught battle: %w", err)
		}
		res.Outcome = OutcomeOutOfBalls
		return res, nil
	}

	// The battle ended. Settle the overworld, then classify from RAM:
	// a catch is the party having GROWN by a wanted species — nothing
	// else counts.
	if err := waitForBattleEnd(m); err != nil {
		return res, err
	}
	state.Snapshot(m, mem)
	party := state.DecodeParty(mem)
	if int(party.Count) == partyBefore+1 && speciesIn(party.Mons[party.Count-1].Species, want) {
		res.Outcome = OutcomeCaught
		res.Species = party.Mons[party.Count-1].Species
		return res, nil
	}
	if targetFainted {
		res.Outcome = OutcomeTargetFainted
		return res, nil
	}
	res.Outcome = OutcomeFled
	return res, nil
}

// waitThrowResult reports whether the battle ended while the result of a
// thrown ball was resolving. It returns (true, nil) once no battle is in
// progress, (false, nil) once the main battle menu is up again (the ball
// broke), and an error if neither happens within the budget.
func waitThrowResult(m *emu.Emu) (bool, error) {
	// The "It broke!" / "Gotcha!" boxes do not auto-advance (measured: a
	// broken ball stalls on that box with no menu up), so each pass without
	// a menu taps A, exactly as Battle's default branch does.
	//
	// A SUCCESSFUL catch is the one place in this loop where A is the wrong
	// button. AddPartyMon asks whether to nickname the catch
	// (pokered/engine/pokemon/add_mon.asm:52 `predef AskName` ->
	// engine/menus/naming_screen.asm), and that prompt is a TWO_OPTION_MENU
	// whose cursor defaults to YES: DisplayTwoOptionMenu
	// (engine/menus/text_box.asm:229) only starts on the second option when
	// BIT_SECOND_MENU_OPTION_DEFAULT is set, and AskName does not set it.
	// wIsInBattle is still non-zero there — AskName reads it — so the
	// battle-in-progress test below is still true, and a blind A opens the
	// naming screen. The keyboard never closes on its own, so the battle
	// never ends and the throw times out with the Pokemon already caught.
	//
	// Answer it the way every other menu in this package is answered:
	// step-and-verify to NO (index 1), never a press count.
	for spent := 0; spent < battleEndSettle; spent += throwPollFrames {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) == nil {
			return true, nil
		}
		if mainMenuUp(m) {
			return false, nil
		}
		if state.DecodeTwoOptionMenu(&mem) != nil {
			if err := SelectMenuItem(m, 1); err != nil {
				return false, fmt.Errorf("declining the nickname prompt: %w", err)
			}
			continue
		}
		// No menu up: a text box or animation. Tap A exactly as Battle's
		// default branch does — inert during animations, and it advances
		// the "It broke!" box.
		m.Tap(emu.A, 3, 7)
		m.StepFrames(throwPollFrames)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		return true, nil
	}
	x, y := playerXY(m)
	return false, fmt.Errorf("neither battle end nor main menu within %d frames: map %02x at (%d,%d)",
		battleEndSettle, m.Peek8(sym.CurMap), x, y)
}

// waitForBattleEnd steps until no battle is in progress and the player is
// controllable again. It returns an error if the battle does not end within
// the budget.
func waitForBattleEnd(m *emu.Emu) error {
	if _, err := m.StepUntil(battleEndSettle, func(m *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return state.DecodeBattle(&mem) == nil && state.Controllable(&mem)
	}); err != nil {
		x, y := playerXY(m)
		return fmt.Errorf("skill: Catch: battle did not end within %d frames: map %02x at (%d,%d)",
			battleEndSettle, m.Peek8(sym.CurMap), x, y)
	}
	return nil
}

// speciesIn reports whether s is one of the wanted species.
func speciesIn(s uint8, want []uint8) bool {
	for _, w := range want {
		if w == s {
			return true
		}
	}
	return false
}
