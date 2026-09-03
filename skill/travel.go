package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// Replan records the world as Travel re-read it after a battle: the map and
// tile the player stands on. It is the point the remainder of the journey
// was planned from.
type Replan struct {
	Map  uint8
	X, Y uint8
}

// TravelResult reports what happened on the way.
type TravelResult struct {
	Battles    int      // wild encounters fought
	Flees      int      // wild encounters fled (S8-7's fight/flee policy)
	Dialogues  int      // text boxes recovered on the way
	BlackedOut bool     // the journey ended in a blackout (a lost battle, or the last mon fainted out of poison)
	Replans    []Replan // one entry per engagement, in order resolved
}

// battleResolution is the outcome of resolving one interrupting battle under
// the journey's policy: either it was fought to a BattleResult, or it was a
// wild encounter that was fled (fled true, outcome empty). A lost fight sets
// outcome to state.ResultLost; a flee can never lose. trainer preserves the
// battle kind after Battle clears wIsInBattle so a loss can be classified.
type battleResolution struct {
	outcome state.BattleResult
	fled    bool
	trainer bool
}

// resolveBattle is the seam between Travel's walk loop and the journey's
// fight/flee policy. It is called once per interrupting battle, with the
// emulator already in that battle, and must leave the player back in the
// overworld before returning.
type resolveBattle func() (battleResolution, error)

// fightOnly resolves every interrupting battle by fighting it. This is the
// policy Travel has always used; a trainer battle is fought exactly as a wild
// one, because there is nothing else to do — you cannot flee a trainer.
func fightOnly(m *emu.Emu, policy MovePolicy) resolveBattle {
	return func() (battleResolution, error) {
		trainer := state.BattleKind(m.Peek8(sym.IsInBattle)) == state.BattleTrainer
		outcome, err := Battle(m, policy)
		return battleResolution{outcome: outcome, trainer: trainer}, err
	}
}

// fleeThenFight resolves an interrupting battle by fleeing it first and only
// falling back to a fight when the game refuses the flee — which it does for
// trainer battles (Flee returns ErrTrainerBattle). This is S8-7's policy:
// wild encounters are fled (S8-4 measured they succeed on the first attempt
// in this ROM, and fleeing skips the damage and the level-up math), while
// trainer battles are fought because they cannot be fled. fleeAttempts bounds
// one battle's flee retries.
func fleeThenFight(m *emu.Emu, policy MovePolicy, fleeAttempts int) resolveBattle {
	return func() (battleResolution, error) {
		if err := Flee(m, fleeAttempts); err != nil {
			if errors.Is(err, ErrTrainerBattle) {
				outcome, berr := Battle(m, policy)
				return battleResolution{outcome: outcome, trainer: true}, berr
			}
			return battleResolution{}, fmt.Errorf("skill: Travel: flee: %w", err)
		}
		return battleResolution{fled: true}, nil
	}
}

// ErrBlackedOut reports that the journey ended in a blackout: a battle was
// lost, or the last mon fainted out of poison while walking (which is not
// a battle — ErrBattle never fires for it). The game fully heals the party
// and respawns the player on the last town's fly-warp spot (a Route 1
// blackout lands on Pallet Town, which has no center at all); the result
// is populated and the journey is over. Re-planning from the respawn spot
// is still the right move, but it is the caller's decision, made with the
// knowledge that the party lost — not a silent continue.
var ErrBlackedOut = errors.New("skill: Travel: blacked out")

// ErrTrainerBlackedOut is the narrower class for a blackout caused by losing
// a mandatory trainer battle. It unwraps to ErrBlackedOut so every existing
// recovery caller keeps working while the agent can distinguish the repeated
// trainer wall from a wild loss or poison wipe.
var ErrTrainerBlackedOut = fmt.Errorf("%w: lost trainer battle", ErrBlackedOut)

func battleBlackoutError(r battleResolution) error {
	if r.trainer {
		return ErrTrainerBlackedOut
	}
	return ErrBlackedOut
}

// blackoutBit is wStatusFlags4's BIT_BATTLE_OVER_OR_BLACKOUT
// (constants/ram_constants.asm:99). The game sets it when a battle ends
// (home/overworld.asm:342) and when poison fainted the whole party out of
// it (engine/events/poison.asm:106, the frame the "blacked out" box
// closes), and clears it on every map entry (EnterMap, home/overworld.asm
// 19-20) and inside HandleBlackOut before the respawn warp. So in the
// overworld it is live only while a blackout transition is in flight: the
// poison case sets it the frame the box closes and it stays set through
// the fade-out until HandleBlackOut clears it, which is the window this
// layer checks.
const blackoutBit = 1 << 5

// ErrDialogueChoice reports that the box that interrupted the walk is a
// two-option prompt and is still unanswered: recovery refuses to answer a
// choice, so the box is up and the game is waiting for an answer this layer
// will not give. The caller decides what to do with it.
type ErrDialogueChoice struct {
	Result DialogueRecoveryResult
}

func (e *ErrDialogueChoice) Error() string {
	return fmt.Sprintf("skill: Travel: text box is a choice and is unanswered: %q", e.Result.Text)
}

// worldStableBudget bounds each settle wait; worldStableFrames is how long
// the world must stand still before Travel trusts a re-read.
const (
	worldStableBudget = 1200
	worldStableFrames = 100
)

// maxDialogueRecoveries bounds how many text boxes Travel recovers on one
// journey. A route has a bounded number of signs and gate NPCs; a journey
// that recovers this many is looping on a box, not paging through the
// world.
const maxDialogueRecoveries = 10

// maxRouteCuts bounds field-move recovery inside one Travel call. Route 9 and
// the Celadon Gym approach each need one tree; four leaves room for a route
// with several legitimate gates while keeping a bad collision classification
// from turning into an unbounded tree-clearing loop.
const maxRouteCuts = 4

func cutRecoverableNavigationError(err error) bool {
	return errors.Is(err, ErrLegUnwalkable) ||
		errors.Is(err, ErrReplanExhausted) ||
		errors.Is(err, world.ErrNoPath)
}

// cutAwareGoTo wraps one Travel journey's GoTo attempts. Only a terminal
// static-path failure is eligible for CUT recovery; battles and dialogue are
// returned immediately to travel's existing resolvers. If the current map has
// a real reachable Cut tree and the party can legally use Cut, the tree is
// removed and the player steps onto the cleared cell before GoTo replans.
// Otherwise the original navigation error is preserved verbatim.
func cutAwareGoTo(m *emu.Emu, romData []byte, dest Destination) func() error {
	cuts := 0
	return func() error {
		for {
			err := GoTo(m, romData, dest)
			if err == nil || errors.Is(err, ErrBattle) || errors.Is(err, ErrDialogueInterrupted) {
				return err
			}
			if cuts >= maxRouteCuts || !cutRecoverableNavigationError(err) {
				return err
			}
			opened, cutErr := cutThroughReachableTree(m, romData)
			if cutErr != nil {
				if errors.Is(cutErr, ErrBattle) || errors.Is(cutErr, ErrDialogueInterrupted) {
					return cutErr
				}
				return fmt.Errorf("skill: Travel: Cut recovery after %v: %w", err, cutErr)
			}
			if !opened {
				return err
			}
			cuts++
		}
	}
}

// Travel walks to dest like GoTo, but resolves the wild encounters, ordinary
// text boxes, and legal Cut-tree route gates that interrupt a route. Each
// encounter is fought with policy; each box is paged closed by
// RecoverDialogue, which never answers a choice. A static path failure may
// trigger CUT only when the current collision grid identifies an actual tree
// tile and RAM says the party has the Cascade Badge plus HM01/Cut.
//
// After every battle the world is re-read from RAM once it has settled, and
// the remainder is planned from that: a win leaves the player on the
// encounter tile, and a blackout rewrites the position to the respawn
// spot (the last town's fly-warp tile) before wCurMap flips, so planning
// from the pre-battle plan — or from inside that pre-flip window — would
// keep walking the map the player is leaving.
//
// ErrBattle and ErrDialogueInterrupted are intercepted. A recovered box is
// retried. A box that is a choice is NOT retried — the choice is unanswered
// and the box is still up, so the next walk would meet it again forever — and
// comes back as *ErrDialogueChoice for the caller to decide. A static failure
// with no usable/reachable Cut tree is returned unchanged.
// A blackout ends the journey with ErrBlackedOut: after a lost battle, and
// after a recovered text box that closed on a non-battle blackout (poison
// fainted the last mon out of it while walking), so a party wiped out
// mid-walk cannot look like a successful arrival. Trainer losses additionally
// satisfy ErrTrainerBlackedOut. maxBattles bounds the fight loop; zero or
// negative is an error, not "unlimited". maxDialogueRecoveries bounds the
// recovery loop the same way.
func Travel(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy, maxBattles int) (TravelResult, error) {
	if maxBattles <= 0 {
		return TravelResult{}, fmt.Errorf("skill: Travel: maxBattles must be > 0, got %d", maxBattles)
	}
	return travel(m, policy, maxBattles,
		cutAwareGoTo(m, romData, dest),
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fightOnly(m, policy),
	)
}

// TravelFlee is Travel with S8-7's fight/flee policy: wild encounters are
// fled instead of fought (skipping the damage and the level-up math), while
// trainer battles are fought because they cannot be fled. It returns the same
// TravelResult as Travel, with Flees counting the fled wilds and Battles the
// fought trainers (and any wild a flee refused). maxBattles bounds total
// engagements (flees and fights alike), exactly as it bounds fights in
// Travel. A lost trainer battle blackouts exactly as in Travel: the error is
// ErrTrainerBlackedOut (and therefore also ErrBlackedOut), the party is fully
// healed at a center, and re-planning from the respawn spot is the caller's
// decision.
func TravelFlee(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy, maxBattles int) (TravelResult, error) {
	return travel(m, policy, maxBattles,
		cutAwareGoTo(m, romData, dest),
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fleeThenFight(m, policy, 5),
	)
}

// travel is the retry loop: walk, resolve what interrupted the walk, walk
// again. goTo, recoverBox and blackout are the resolvers; Travel wires them
// to GoTo/Cut recovery, RecoverDialogue and the wStatusFlags4 blackout bit,
// and the tests drive the loop with fakes instead of an emulator.
func travel(m *emu.Emu, policy MovePolicy, maxBattles int, goTo func() error, recoverBox func() DialogueRecoveryResult, blackout func() bool, resolveBattle resolveBattle) (TravelResult, error) {
	var res TravelResult
	for {
		err := goTo()
		if err == nil {
			return res, nil
		}
		switch {
		case errors.Is(err, ErrBattle):
			// The bound is on engagements (fights and flees alike): it caps how
			// long the walk may be interrupted, whatever the policy does with each
			// encounter.
			if res.Battles+res.Flees >= maxBattles {
				return res, fmt.Errorf("skill: Travel: still interrupted after %d engagement(s) (maxBattles): %v",
					maxBattles, err)
			}
			pre := currentWorld(m)
			r, berr := resolveBattle()
			if berr != nil {
				return res, fmt.Errorf("skill: Travel: battle %d: %w", res.Battles+res.Flees+1, berr)
			}
			if r.fled {
				res.Flees++
			} else {
				res.Battles++
			}
			lost := r.outcome == state.ResultLost
			res.Replans = append(res.Replans, settleWorld(m, pre, lost))
			if lost {
				// A blackout ends the journey. Losing was once a silent
				// continue — "the next pass re-plans from the Pokemon
				// Center" — which is wrong in composition: the respawn
				// fully heals and cures the party (ResetStatusAndHalveMoney
				// OnBlackout ends in HealParty), the caller never learned a
				// loss had happened, walked back into the same hazard
				// (re-poisoning on the route), and died again (the
				// 2026-08-28 death spiral: Viridian, poisoned, 2 HP, two
				// deaths, nothing to see either fact). Re-planning from the
				// respawn spot is still the right move, but it is the
				// caller's decision, made with the knowledge that the party
				// lost. Trainer losses preserve that narrower cause too.
				res.BlackedOut = true
				return res, battleBlackoutError(r)
			}
		case errors.Is(err, ErrDialogueInterrupted):
			if res.Dialogues >= maxDialogueRecoveries {
				return res, fmt.Errorf("skill: Travel: still interrupted by a text box after %d recoveries: %v",
					maxDialogueRecoveries, err)
			}
			res.Dialogues++
			rec := recoverBox()
			switch rec.Stop {
			case DialogueChoiceRequired, DialogueMenuOpen:
				// The choice is unanswered, or a menu is up that this layer
				// will not operate, and either way the screen is still
				// there — so the next walk returns the same interruption
				// forever. Return the typed outcome and let the caller
				// decide; retrying here would loop.
				return res, &ErrDialogueChoice{Result: rec}
			case DialogueBudgetExhausted:
				// The box did not clear within the budget and is still up,
				// so retrying would only meet it again. Report it with the
				// text that was on screen.
				return res, fmt.Errorf("skill: Travel: text box did not clear within the recovery budget: %q", rec.Text)
			case DialogueRecovered:
				if blk := blackout(); blk {
					// The box that just closed was the blackout's own text:
					// poison fainted the last mon out of it while walking.
					// That is not a battle — ErrBattle never fired — it
					// surfaced as an ordinary box, and the game set the
					// blackout bit the frame the box closed; it stays set
					// through the fade-out until HandleBlackOut clears it on
					// the respawn warp, which has not happened yet. A party
					// wiped out mid-walk must not look like a successful
					// arrival: treat it exactly like a lost battle.
					res.BlackedOut = true
					return res, ErrBlackedOut
				}
				// recovered: the box is closed; the next pass re-plans from
				// where the walk stopped.
			case DialogueUnexpectedMode:
				// unexpected mode: a battle the box led into is in progress;
				// the next pass's GoTo normalizes it to ErrBattle, and the
				// battle branch fights it.
			}
		default:
			return res, err
		}
	}
}

// currentWorld reads the map and tile the player stands on from RAM.
func currentWorld(m *emu.Emu) Replan {
	return Replan{m.Peek8(sym.CurMap), m.Peek8(sym.XCoord), m.Peek8(sym.YCoord)}
}

// settleWorld steps until the (map, x, y) triple has stood still for
// worldStableFrames consecutive frames and returns that settled world.
// On a loss it first waits for the map to change: a blackout lands the
// position on the center's spawn tile before wCurMap flips, and that
// pre-flip window is itself stable, so a plain stability wait would settle
// on the stale map (the measured "step down blocked at (5,6)" walked a
// 0x0C plan while on 0x00).
func settleWorld(m *emu.Emu, pre Replan, lost bool) Replan {
	if lost {
		if _, err := m.StepUntil(worldStableBudget, func(m *emu.Emu) bool {
			return m.Peek8(sym.CurMap) != pre.Map
		}); err != nil {
			// ponytail: blackout transition longer than worldStableBudget ->
			// fall through with the last read (today's behavior) rather than
			// failing; raise worldStableBudget if that is ever measured.
		}
	}
	last := currentWorld(m)
	stable := 0
	for i := 0; i < worldStableBudget; i++ {
		m.StepFrame()
		cur := currentWorld(m)
		if cur == last {
			stable++
			if stable >= worldStableFrames {
				return cur
			}
		} else {
			stable = 0
		}
		last = cur
	}
	return last
}
