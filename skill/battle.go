package skill

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// MovePolicy chooses which move slot to use. It is given the decoded
// battle state and returns an index into BattleState.Moves. Returning an
// index that is not in Usable() is a programming error and Battle will
// report it rather than pressing anything.
//
// This is the seam where a learned policy eventually plugs in: Battle
// decodes the state, asks the policy for a slot, and presses exactly that
// slot. The default policy is deterministic, so tests never call a model.
type MovePolicy func(state.BattleState) int

// FirstUsableMove is the default policy: the lowest-numbered usable move
// slot. Using BattleState.Usable keeps the default aligned with Battle's
// legality check, including disabled moves and PP exhaustion.
func FirstUsableMove(b state.BattleState) int {
	usable := b.Usable()
	if len(usable) == 0 {
		return -1
	}
	return usable[0]
}

// ErrNoUsableMove reports that the active Pokémon has no selectable move.
// Battle normally recovers by switching to a live party member with PP or,
// when none exists, by backing out so the ROM can select STRUGGLE. The error
// remains a defensive diagnostic for menu states that refuse both paths.
var ErrNoUsableMove = errors.New("skill: no usable move")

// ErrForcedChoiceStuck reports that the post-refusal "choose a Pokémon"
// screen (wForcePlayerToChooseMon, set when a trainer refuses RUN — see
// ErrTrainerBattle) did not clear within forcedChoiceCap round-trips.
// RESOLVED 2026-08-31 for the normal case (SLICE10-CANDIDATES.md item 19,
// the Viridian Forest wedge): the correct sequence — pick the only live
// slot, B out of the SWITCH/STATS box, B again on the party list, then
// selectFightEntry to reach FIGHT from wherever the cursor landed — is
// CONFIRMED against pret/pokered's DisplayBattleMenu / PartyMenuInit and
// verified live against the checkpointed repro (won the battle it used to
// hang on for the full battleFrameCap). This error is now a backstop should
// the sequence not resolve for a reason this session did not encounter —
// e.g. a different party composition — so the failure is fast and
// diagnosable instead of silently spinning for 60000 frames.
var ErrForcedChoiceStuck = errors.New("skill: Battle: the post-refusal Pokémon-choice screen did not clear")

// forcedChoiceCap bounds how many times the switch/party-list round-trip
// (case switchBoxUp / battleSwitchMenuUp below) may repeat before giving up
// with ErrForcedChoiceStuck.
const forcedChoiceCap = 5

// voluntarySwitchCap is a second anti-loop bound on policy-driven switches.
// Equivalent candidates already fail chooseTacticalSwitch's material-gain
// threshold; this cap also contains pathological stat-reset matchups.
const voluntarySwitchCap = 4

// Frame budgets for Battle. They are upper bounds, not measured timings: a
// real turn (menu + move + resolution) is a few hundred frames and a whole
// battle a few thousand. The stall cap exists so a stuck battle fails
// loudly instead of hanging the suite. It is measured since the last HP or
// species change, not since the battle began: a PP-exhausted STRUGGLE war
// against a high-level trainer is slow but still resolving, and cutting it
// off turns an ordinary blackout into a dirty objective boundary.
// battleFrameCap is only an absolute backstop.
const (
	battleStallCap  = 60000  // frames without any HP/species change
	battleFrameCap  = 600000 // total frames for the whole battle
	moveMenuBudget  = 500    // wait for the move/main menu transition
	moveCloseBudget = 500    // wait for the move menu to close after a move
	settleBudget    = 3000   // wait for controllable after the battle ends
)

// mainMenuMax is the wMaxMenuItem of the FIGHT/ITEM/PKMN/RUN menu. The move
// menu uses wNumMovesMinusOne+2 (>= 2), so this value identifies the main
// battle menu unambiguously.
const mainMenuMax = 1

// Battle fights the current battle to completion using policy, and returns
// how it ended. It returns an error if no battle is in progress when called.
//
// The battle is driven as a state machine. The FIGHT/ITEM/PKMN/RUN menu is
// identified by wMaxMenuItem == 1; the move menu by wMaxMenuItem >= 2. Text
// boxes and animations (which carry a stale wMaxMenuItem) are advanced with
// A. Before committing to FIGHT, Battle can switch to a materially stronger
// party matchup, spend a bounded medicine turn, or recover from PP exhaustion.
// If the whole party is PP-dead, the ROM's own legal STRUGGLE fallback is
// allowed to run rather than leaving the caller trapped inside a battle.
//
// Battle also answers the forced switch after a faint in a wild battle: the
// "Use next #MON?" prompt is answered YES (NO is an escape attempt) and the
// best live replacement for the current opponent is sent out. Level-up move
// learning is deliberate: DecideNaturalMove can decline a bad move or choose
// the weakest replaceable slot from the full move-set score. The chosen move
// is then positively verified from party RAM before Battle is allowed to
// finish. The prompt plays during GainExperience while wIsInBattle is still
// set, so it reaches this loop; Train never sees it.
// The OTHER half of the party menu — the voluntary switch opened by the
// player through the POKéMON branch — is driven by SwitchActive. Battle now
// uses that same verified primitive for tactical switches and PP recovery; it
// still backstops an unbidden menu after a trainer's RUN refusal (see
// ErrForcedChoiceStuck).
//
// If the game reaches any other state Battle does not handle, the frame cap
// trips and Battle fails loudly. Losing is a result, not an error: a blackout
// returns ResultLost with a nil error. Recovering from a blackout is out of
// scope.
//
// zbatDebug is read once at init, not per frame: Go 1.27's test harness logs
// every os.Getenv call to the test log (measured: a per-frame getenv produced
// 42MB of "getenv ZBAT" lines in one nine-minute run).
var zbatDebug = os.Getenv("ZBAT") != ""

// BattleOptions adds narrowly-scoped battle behavior for callers that need a
// deliberate opening switch. Ordinary Battle uses the zero value and retains
// the normal tactical policy.
type BattleOptions struct {
	OpeningTrainingSwitch bool
	MinTrainingCarryLevel uint8
}

func Battle(m *emu.Emu, policy MovePolicy) (state.BattleResult, error) {
	return BattleWithOptions(m, policy, BattleOptions{})
}

func BattleWithOptions(m *emu.Emu, policy MovePolicy, options BattleOptions) (state.BattleResult, error) {
	if policy == nil {
		return 0, errors.New("skill: Battle: nil policy")
	}
	executionDecoder, err := battleExecutionDecoderFor(m)
	if err != nil {
		return 0, fmt.Errorf("skill: Battle: %w", err)
	}
	menuDecoder, err := menuDecoderFor(m)
	if err != nil {
		return 0, fmt.Errorf("skill: Battle: %w", err)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		x, y := playerXY(m)
		return 0, fmt.Errorf("skill: Battle: no battle in progress on map %02x at (%d,%d)",
			m.Peek8(sym.CurMap), x, y)
	}

	startFrame := m.FrameCount()
	progressFrame := startFrame
	var lastProgress battleProgress

	// The move-learning episode is tracked across loop passes. lastForgetSlot
	// is the slot just picked in the forget list; triedForgets records a move
	// the ROM explicitly bounced as an HM technique. pendingLearn* is the
	// positive postcondition: the offered move must appear in that exact party
	// slot before Battle exits.
	var lastForgetSlot = -1
	var triedForgets map[uint8]bool
	pendingLearnMove := uint16(0)
	pendingLearnSlot := -1
	pendingLearnPartySlot := -1
	forcedChoiceVisits := 0
	itemUses := 0
	voluntarySwitches := 0
	openingTrainingSwitch := options.OpeningTrainingSwitch
	// pendingTryLearn remembers that the "<NAME> is trying to learn <MOVE>"
	// text was seen. That message is long enough to scroll off the 4-line
	// battle text box before the YES/NO cursor is drawn (the cursor appears
	// only after PrintText finishes the whole message), so by the time the
	// prompt is actually answerable the "trying to learn" marker is gone and
	// twoOptionPromptUp no longer matches. Without this flag the loop falls
	// to the default blind-Tap(A) branch, which confirms YES and strands the
	// policy on the forget menu with no legal replacement.
	pendingTryLearn := false

	for {
		if int(m.FrameCount()-startFrame) > battleFrameCap {
			return stuckError(m, fmt.Sprintf("exceeded %d-frame cap", battleFrameCap))
		}
		if int(m.FrameCount()-progressFrame) > battleStallCap {
			return stuckError(m, fmt.Sprintf("no HP or species change for %d frames", battleStallCap))
		}

		state.Snapshot(m, &mem)
		execution := executionDecoder.DecodeBattleExecution(m)
		if bs := state.DecodeBattle(&mem); bs != nil {
			if p := progressOf(bs); p != lastProgress {
				lastProgress, progressFrame = p, m.FrameCount()
			}
		}
		if pendingLearnMove != 0 && pendingLearnSlot >= 0 && pendingLearnPartySlot >= 0 {
			if execution.MoveLearned(pendingLearnPartySlot, pendingLearnSlot, pendingLearnMove) {
				if zbatDebug {
					fmt.Printf("zbat move-learn verified move=%d party-slot=%d move-slot=%d\n",
						pendingLearnMove, pendingLearnPartySlot, pendingLearnSlot)
				}
				pendingLearnMove = 0
				pendingLearnSlot = -1
				pendingLearnPartySlot = -1
				lastForgetSlot = -1
				triedForgets = nil
			}
		}
		if zbatDebug {
			if bs := state.DecodeBattle(&mem); bs != nil {
				fmt.Printf("zbat f=%6d max=%d cur=%d me=%d/%d enemy=%d/%d moves=%v items=%d/%d switches=%d/%d | %s\n",
					m.FrameCount(), m.Peek8(sym.MaxMenuItem), m.Peek8(sym.CurrentMenuItem),
					bs.ActiveHP, bs.ActiveMaxHP, bs.EnemyHP, bs.EnemyMaxHP, bs.Moves,
					itemUses, battleItemUseCap, voluntarySwitches, voluntarySwitchCap,
					strings.Join(strings.Fields(state.ScreenText(&mem)), " "))
			}
		}
		if state.DecodeBattle(&mem) == nil {
			if pendingLearnMove != 0 {
				return 0, fmt.Errorf("skill: Battle: natural move %d was accepted for party slot %d move slot %d but the resulting move set was never verified",
					pendingLearnMove, pendingLearnPartySlot, pendingLearnSlot)
			}
			if zbatDebug {
				fmt.Printf("zbat EXIT f=%d inBattle=%#02x rawResult=%#02x\n",
					m.FrameCount(), m.Peek8(sym.IsInBattle), m.Peek8(sym.BattleResult))
			}
			// Read the result at the battle boundary: settling walks through
			// a blackout respawn, which clears wBattleResult and would report
			// the loss as a win.
			result := state.DecodeBattleResult(&mem)
			if err := settleAfterBattle(m, &mem); err != nil {
				return 0, err
			}
			return result, nil
		}

		switch {
		case execution.Phase == game.BattleExecutionMoveMenu || execution.Phase == game.BattleExecutionMoveDisabled:
			if execution.Phase == game.BattleExecutionMoveDisabled {
				m.Tap(emu.A, 3, 7)
				if _, err := m.StepUntil(moveMenuBudget, func(m *emu.Emu) bool {
					return executionDecoder.DecodeBattleExecution(m).Phase != game.BattleExecutionMoveDisabled
				}); err != nil {
					return menuError(m, "clear the disabled-move refusal", err)
				}
				state.Snapshot(m, &mem)
				execution = executionDecoder.DecodeBattleExecution(m)
			}
			bs := state.DecodeBattle(&mem)
			if bs == nil {
				continue
			}
			usable := bs.Usable()
			if len(usable) == 0 {
				if slot, ok := ppRecoverySlot(&mem); ok {
					m.Tap(emu.B, 3, 7)
					if _, err := m.StepUntil(moveMenuBudget, func(m *emu.Emu) bool {
						return executionDecoder.DecodeBattleExecution(m).Phase == game.BattleExecutionMainMenu
					}); err != nil {
						return menuError(m, "back out of unusable move menu for PP switch", err)
					}
					if zbatDebug {
						fmt.Printf("zbat resource=SWITCH action=emergency reason=no-usable-move slot=%d\n", slot)
					}
					if err := SwitchActive(m, slot); err != nil {
						return menuError(m, "switch to party member with PP", err)
					}
					continue
				}
				m.Tap(emu.B, 3, 7)
				if _, err := m.StepUntil(moveMenuBudget, func(m *emu.Emu) bool {
					return executionDecoder.DecodeBattleExecution(m).Phase == game.BattleExecutionMainMenu
				}); err != nil {
					x, y := playerXY(m)
					return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d) battle %+v: cannot leave unusable move menu: %w",
						m.Peek8(sym.CurMap), x, y, bs, ErrNoUsableMove)
				}
				continue
			}
			slot := policy(*bs)
			if !containsInt(usable, slot) {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d) battle %+v: policy returned slot %d, usable %v",
					m.Peek8(sym.CurMap), x, y, bs, slot, usable)
			}
			observeMove(m, *bs, slot)
			if err := SelectMenuItem(m, slot+1); err != nil {
				return menuError(m, "select move", err)
			}
			_, _ = m.StepUntil(moveCloseBudget, func(m *emu.Emu) bool {
				phase := executionDecoder.DecodeBattleExecution(m).Phase
				return phase != game.BattleExecutionMoveMenu && phase != game.BattleExecutionMoveDisabled
			})

		case execution.Phase == game.BattleExecutionMainMenu:
			if openingTrainingSwitch {
				bs := state.DecodeBattle(&mem)
				if bs != nil {
					decision := chooseTrainingCarrySwitch(m.ROM(), &mem, *bs, options.MinTrainingCarryLevel)
					if decision.Switch {
						if zbatDebug {
							fmt.Printf("zbat resource=SWITCH action=training reason=%s active={%s} candidate={%s}\n",
								decision.Reason, decision.Active.String(), decision.Candidate.String())
						}
						if err := SwitchActive(m, decision.Slot); err != nil {
							return menuError(m, "switch-training carry", err)
						}
						openingTrainingSwitch = false
						voluntarySwitches++
						continue
					}
				}
				// Train checks the same carry predicate before entering a wild
				// encounter. If the live battle no longer has one (for example a
				// status/HP transition landed between steps), fall through to the
				// ordinary battle policy so Battle still resolves to a clean
				// boundary rather than stranding the emulator mid-fight.
				openingTrainingSwitch = false
			}

			if bs := state.DecodeBattle(&mem); bs != nil && len(bs.Usable()) == 0 {
				if slot, ok := ppRecoverySlot(&mem); ok {
					if zbatDebug {
						fmt.Printf("zbat resource=SWITCH action=emergency reason=no-usable-move slot=%d\n", slot)
					}
					if err := SwitchActive(m, slot); err != nil {
						return menuError(m, "switch to party member with PP", err)
					}
					continue
				}
				if zbatDebug && !livePartyHasCurrentPP(&mem) {
					fmt.Printf("zbat resource=STRUGGLE reason=all-live-party-pp-exhausted\n")
				}
			}

			// A deliberate switch-training battle already chose the best carry
			// for this exact opponent. Do not tactically rotate through additional
			// party members afterward: every extra participant further splits the
			// trainee's XP and defeats the estimator's two-participant contract.
			if !options.OpeningTrainingSwitch && voluntarySwitches < voluntarySwitchCap {
				if bs := state.DecodeBattle(&mem); bs != nil && len(bs.Usable()) > 0 {
					decision := chooseTacticalSwitch(m.ROM(), &mem, *bs)
					if decision.Switch {
						if zbatDebug {
							fmt.Printf("zbat resource=SWITCH action=voluntary reason=%s active={%s} candidate={%s}\n",
								decision.Reason, decision.Active.String(), decision.Candidate.String())
						}
						if err := SwitchActive(m, decision.Slot); err != nil {
							return menuError(m, "tactical party switch", err)
						}
						voluntarySwitches++
						continue
					}
					if zbatDebug && decision.Legal && decision.Slot >= 0 {
						fmt.Printf("zbat resource=SWITCH action=stay reason=%s active={%s} candidate={%s}\n",
							decision.Reason, decision.Active.String(), decision.Candidate.String())
					}
				}
			}

			if itemUses < battleItemUseCap {
				if choice, ok := chooseBattleMedicine(&mem); ok {
					if zbatDebug {
						fmt.Printf("zbat resource=ITEM item=%#02x slot=%d reason=%s\n", choice.Item, choice.Slot, choice.Reason)
					}
					if err := UseBattleMedicine(m, choice.Item, choice.Slot); err != nil {
						return menuError(m, "use battle medicine", err)
					}
					itemUses++
					continue
				}
			}

			if err := selectFightEntry(m); err != nil {
				return menuError(m, "select FIGHT", err)
			}
			if err := SelectMenuItem(m, 0); err != nil {
				return menuError(m, "select FIGHT", err)
			}

		case execution.Phase == game.BattleExecutionHMForgetRejected && lastForgetSlot >= 0:
			learner, learnerSlot, ok := naturalMoveLearner(execution)
			if !ok || lastForgetSlot >= len(learner.Moves) || learner.Moves[lastForgetSlot] > 0xff {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d): invalid move-learning party slot %d during HM rejection",
					m.Peek8(sym.CurMap), x, y, learnerSlot)
			}
			if triedForgets == nil {
				triedForgets = map[uint8]bool{}
			}
			rejectedMove := uint8(learner.Moves[lastForgetSlot])
			triedForgets[rejectedMove] = true
			if zbatDebug {
				fmt.Printf("zbat move-learn action=hm-rejected party-slot=%d slot=%d move=%d\n", learnerSlot, lastForgetSlot, rejectedMove)
			}
			lastForgetSlot = -1
			pendingLearnSlot = -1
			pendingLearnPartySlot = -1
			m.Tap(emu.A, 3, 7)
			if _, err := m.StepUntil(moveMenuBudget, func(m *emu.Emu) bool {
				return executionDecoder.DecodeBattleExecution(m).Phase != game.BattleExecutionHMForgetRejected
			}); err != nil {
				return menuError(m, "dismiss HM move-forget refusal", err)
			}

		case execution.Phase == game.BattleExecutionForgetMove:
			if !execution.ForgetReady {
				m.StepFrame()
				continue
			}
			if !execution.InBattle {
				continue
			}
			if lastForgetSlot >= 0 {
				m.StepFrame()
				continue
			}
			offered := execution.OfferedMove
			decision, learnerSlot, ok := naturalMoveDecisionForLearner(execution, m.ROM(), offered, triedForgets)
			if !ok {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d): invalid move-learning party slot %d",
					m.Peek8(sym.CurMap), x, y, learnerSlot)
			}
			if !decision.Learn || decision.ReplaceSlot < 0 {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d): accepted natural move %d for party slot %d but no legal strategic replacement remains: %s",
					m.Peek8(sym.CurMap), x, y, offered, learnerSlot, decision.Reason)
			}
			if zbatDebug {
				fmt.Printf("zbat move-learn action=replace party-slot=%d %s\n", learnerSlot, decision.Reason)
			}
			slot := decision.ReplaceSlot
			pendingLearnMove = uint16(decision.Offered)
			pendingLearnSlot = slot
			pendingLearnPartySlot = learnerSlot
			if err := selectForgetSlot(m, slot); err != nil {
				return menuError(m, "select move to forget", err)
			}
			lastForgetSlot = slot

		case execution.Phase == game.BattleExecutionTrainerSwitch:
			if _, ready := menuDecoder.DecodeTwoOption(m); !ready {
				m.StepFrame()
				continue
			}
			if err := selectTwoOption(m, 1); err != nil {
				return menuError(m, "decline trainer switch", err)
			}

		case execution.Phase == game.BattleExecutionAbandonLearn:
			if _, ready := menuDecoder.DecodeTwoOption(m); !ready {
				m.StepFrame()
				continue
			}
			if err := SelectMenuItem(m, 0); err != nil {
				return menuError(m, "confirm decline of natural move", err)
			}

		case execution.Phase == game.BattleExecutionUseNextPrompt ||
			execution.Phase == game.BattleExecutionTryLearnPrompt ||
			pendingTryLearn && func() bool {
				_, ready := menuDecoder.DecodeTwoOption(m)
				return ready
			}():
			if execution.Phase == game.BattleExecutionTryLearnPrompt {
				pendingTryLearn = true
			}
			if _, ready := menuDecoder.DecodeTwoOption(m); !ready {
				m.Tap(emu.A, 3, 7)
				continue
			}
			choice := 0
			if pendingTryLearn {
				if execution.InBattle {
					offered := execution.OfferedMove
					decision, learnerSlot, ok := naturalMoveDecisionForLearner(execution, m.ROM(), offered, nil)
					if !ok {
						x, y := playerXY(m)
						return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d): invalid move-learning party slot %d",
							m.Peek8(sym.CurMap), x, y, learnerSlot)
					}
					if !decision.Learn {
						choice = 1
					}
					if zbatDebug {
						action := "learn"
						if !decision.Learn {
							action = "decline"
						}
						fmt.Printf("zbat move-learn action=%s party-slot=%d offered=%d reason=%s\n", action, learnerSlot, offered, decision.Reason)
					}
					pendingTryLearn = false
				}
			}
			if err := selectTwoOption(m, choice); err != nil {
				return menuError(m, "answer two-option prompt", err)
			}

		case partyMenuUp(m):
			var s state.Mem
			state.Snapshot(m, &s)
			slot := firstLivePartySlot(&s)
			var replacement switchEvaluation
			if bs := state.DecodeBattle(&s); bs != nil {
				if bestSlot, best := bestReplacementSlot(m.ROM(), &s, *bs); bestSlot >= 0 {
					slot, replacement = bestSlot, best
				}
			}
			if slot < 0 {
				m.StepFrame()
				continue
			}
			if zbatDebug && replacement.Slot >= 0 {
				fmt.Printf("zbat resource=SWITCH action=forced reason=best-live-replacement candidate={%s}\n", replacement.String())
			}
			if err := SelectPartySlot(m, slot); err != nil {
				return menuError(m, "select party slot", err)
			}

		case switchBoxUp(m):
			m.Tap(emu.B, 3, 7)

		case battleSwitchMenuUp(m):
			forcedChoiceVisits++
			if forcedChoiceVisits > forcedChoiceCap {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d): %w", m.Peek8(sym.CurMap), x, y, ErrForcedChoiceStuck)
			}
			if forcedChoiceVisits == 1 {
				var s state.Mem
				state.Snapshot(m, &s)
				slot := firstLivePartySlot(&s)
				if slot < 0 {
					m.StepFrame()
					continue
				}
				if err := SelectPartySlot(m, slot); err != nil {
					return menuError(m, "select party slot (forced choice)", err)
				}
			} else {
				m.Tap(emu.B, 3, 7)
			}

		default:
			m.Tap(emu.A, 3, 7)
		}
	}
}

// firstLivePartySlot remains Gen-I battle strategy state for now. UI surface
// classification and move-learning execution above are profile-driven.
func firstLivePartySlot(mem *state.Mem) int {
	party := state.DecodeParty(mem)
	for i, mon := range party.Mons {
		if !mon.Fainted() {
			return i
		}
	}
	return -1
}

func battleScreenHas(m *emu.Emu, marker string) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return strings.Contains(state.ScreenText(&mem), marker)
}

const settleStableFrames = 20

// ErrCampaignComplete reports that a battle's aftermath was the game's ending:
// the main story is now complete and control will not return to the caller's
// walk. The run goal check, not the interrupted objective, owns what follows.
var ErrCampaignComplete = errors.New("skill: Battle: campaign complete; the ending never returns control")

// battleProgress is the part of a battle that must keep changing while the
// fight is still resolving. Menus and text never change it; a landed hit, a
// heal, a faint or a switch does.
type battleProgress struct {
	activeHP, enemyHP           uint16
	activeSpecies, enemySpecies uint8
}

func progressOf(bs *state.BattleState) battleProgress {
	return battleProgress{bs.ActiveHP, bs.EnemyHP, bs.ActiveSpecies, bs.EnemySpecies}
}

func settleAfterBattle(m *emu.Emu, mem *state.Mem) error {
	startFrame := m.FrameCount()
	stable := 0
	for int(m.FrameCount()-startFrame) < settleBudget {
		state.Snapshot(m, mem)
		if state.Controllable(mem) {
			stable++
			if stable >= settleStableFrames {
				return nil
			}
		} else {
			stable = 0
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	// The Champion's defeat hands the game to its ending, which never returns
	// control. Whichever skill fought that battle, the League adapter owns
	// driving the ending to its durable completion bit.
	if leagueFacts(mem).LeagueChampionDefeated {
		if err := finishHallOfFame(m); err != nil {
			return err
		}
		return ErrCampaignComplete
	}
	x, y := playerXY(m)
	return fmt.Errorf("skill: Battle: not controllable %d frames after the battle ended: map %02x at (%d,%d)",
		settleBudget, m.Peek8(sym.CurMap), x, y)
}

func stuckError(m *emu.Emu, detail string) (state.BattleResult, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	x, y := playerXY(m)
	bs := "<none>"
	if b := state.DecodeBattle(&mem); b != nil {
		bs = fmt.Sprintf("%+v", b)
	}
	return 0, fmt.Errorf("skill: Battle: %s: map %02x at (%d,%d) battle %s",
		detail, m.Peek8(sym.CurMap), x, y, bs)
}

func menuError(m *emu.Emu, detail string, err error) (state.BattleResult, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	x, y := playerXY(m)
	bs := "<none>"
	if b := state.DecodeBattle(&mem); b != nil {
		bs = fmt.Sprintf("%+v", b)
	}
	return 0, fmt.Errorf("skill: Battle: %s: map %02x at (%d,%d) battle %s: %w",
		detail, m.Peek8(sym.CurMap), x, y, bs, err)
}

func containsInt(slice []int, x int) bool {
	for _, v := range slice {
		if v == x {
			return true
		}
	}
	return false
}
