package skill

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
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
// battle a few thousand. The total cap exists so a stuck battle fails
// loudly instead of hanging the suite.
const (
	battleFrameCap  = 60000 // total frames for the whole battle
	moveMenuBudget  = 500   // wait for the move/main menu transition
	moveCloseBudget = 500   // wait for the move menu to close after a move
	settleBudget    = 3000  // wait for controllable after the battle ends
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

func Battle(m *emu.Emu, policy MovePolicy) (state.BattleResult, error) {
	if policy == nil {
		return 0, errors.New("skill: Battle: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		x, y := playerXY(m)
		return 0, fmt.Errorf("skill: Battle: no battle in progress on map %02x at (%d,%d)",
			m.Peek8(sym.CurMap), x, y)
	}

	startFrame := m.FrameCount()

	// The move-learning episode is tracked across loop passes. lastForgetSlot
	// is the slot just picked in the forget list; triedForgets records a move
	// the ROM explicitly bounced as an HM technique. pendingLearn* is the
	// positive postcondition: the offered move must appear in that exact party
	// slot before Battle exits.
	var lastForgetSlot = -1
	var triedForgets map[uint8]bool
	pendingLearnMove := uint8(0)
	pendingLearnSlot := -1
	pendingLearnPartySlot := -1
	forcedChoiceVisits := 0
	itemUses := 0
	voluntarySwitches := 0
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

		state.Snapshot(m, &mem)
		if pendingLearnMove != 0 && pendingLearnSlot >= 0 && pendingLearnPartySlot >= 0 {
			party := state.DecodeParty(&mem)
			if pendingLearnPartySlot < len(party.Mons) && party.Mons[pendingLearnPartySlot].Moves[pendingLearnSlot] == pendingLearnMove {
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
			if err := settleAfterBattle(m, &mem); err != nil {
				return 0, err
			}
			state.Snapshot(m, &mem)
			return state.DecodeBattleResult(&mem), nil
		}

		switch {
		case moveMenuUp(m):
			if disabledMoveRefusalUp(m) {
				m.Tap(emu.A, 3, 7)
				if _, err := m.StepUntil(moveMenuBudget, func(m *emu.Emu) bool {
					return !disabledMoveRefusalUp(m)
				}); err != nil {
					return menuError(m, "clear the disabled-move refusal", err)
				}
				state.Snapshot(m, &mem)
			}
			bs := state.DecodeBattle(&mem)
			if bs == nil {
				continue
			}
			usable := bs.Usable()
			if len(usable) == 0 {
				if slot, ok := ppRecoverySlot(&mem); ok {
					m.Tap(emu.B, 3, 7)
					if _, err := m.StepUntil(moveMenuBudget, mainMenuUp); err != nil {
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
				if _, err := m.StepUntil(moveMenuBudget, mainMenuUp); err != nil {
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
			if err := SelectMenuItem(m, slot+1); err != nil {
				return menuError(m, "select move", err)
			}
			_, _ = m.StepUntil(moveCloseBudget, func(m *emu.Emu) bool {
				return !moveMenuUp(m)
			})

		case mainMenuUp(m):
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

			if voluntarySwitches < voluntarySwitchCap {
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

		case moveLearnForgetRejected(lastForgetSlot, state.ScreenText(&mem)):
			learner, learnerSlot, ok := naturalMoveLearner(&mem)
			if !ok || lastForgetSlot >= len(learner.Moves) {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d): invalid move-learning party slot %d during HM rejection",
					m.Peek8(sym.CurMap), x, y, learnerSlot)
			}
			if triedForgets == nil {
				triedForgets = map[uint8]bool{}
			}
			rejectedMove := learner.Moves[lastForgetSlot]
			triedForgets[rejectedMove] = true
			if zbatDebug {
				fmt.Printf("zbat move-learn action=hm-rejected party-slot=%d slot=%d move=%d\n", learnerSlot, lastForgetSlot, rejectedMove)
			}
			lastForgetSlot = -1
			pendingLearnSlot = -1
			pendingLearnPartySlot = -1
			m.Tap(emu.A, 3, 7)
			if _, err := m.StepUntil(moveMenuBudget, func(m *emu.Emu) bool {
				return !battleScreenHas(m, hmCantDeleteMarker)
			}); err != nil {
				return menuError(m, "dismiss HM move-forget refusal", err)
			}

		case forgetMenuUp(m):
			if state.DecodeMenu(&mem).Max != 3 {
				m.StepFrame()
				continue
			}
			if state.DecodeBattle(&mem) == nil {
				continue
			}
			if lastForgetSlot >= 0 {
				m.StepFrame()
				continue
			}
			offered := m.Peek8(sym.MoveNum)
			decision, learnerSlot, ok := naturalMoveDecisionForLearner(&mem, m.ROM(), offered, triedForgets)
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
			pendingLearnMove = offered
			pendingLearnSlot = slot
			pendingLearnPartySlot = learnerSlot
			if err := selectForgetSlot(m, slot); err != nil {
				return menuError(m, "select move to forget", err)
			}
			lastForgetSlot = slot

		case trainerSwitchPromptUp(m):
			var s state.Mem
			state.Snapshot(m, &s)
			if state.DecodeTwoOptionMenu(&s) == nil {
				m.StepFrame()
				continue
			}
			if err := selectTwoOption(m, 1); err != nil {
				return menuError(m, "decline trainer switch", err)
			}

		case abandonLearnPromptUp(m):
			var s state.Mem
			state.Snapshot(m, &s)
			if state.DecodeTwoOptionMenu(&s) == nil {
				m.StepFrame()
				continue
			}
			if err := SelectMenuItem(m, 0); err != nil {
				return menuError(m, "confirm decline of natural move", err)
			}

		case twoOptionPromptUp(m) || (pendingTryLearn && twoOptionCursorUp(m)):
			var s state.Mem
			state.Snapshot(m, &s)
			text := state.ScreenText(&s)
			if strings.Contains(text, tryLearnMarker) {
				pendingTryLearn = true
			}
			if state.DecodeTwoOptionMenu(&s) == nil {
				m.Tap(emu.A, 3, 7)
				continue
			}
			choice := 0
			if pendingTryLearn {
				if state.DecodeBattle(&s) != nil {
					offered := m.Peek8(sym.MoveNum)
					decision, learnerSlot, ok := naturalMoveDecisionForLearner(&s, m.ROM(), offered, nil)
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

// Battle menus are identified by what the game has drawn into wTileMap,
// because wFontLoaded — which every overworld skill relies on — is MEASURED
// to stay 0 for the whole of a battle. Battle text does not go through the
// overworld text engine. Gating on it made this whole state machine dead
// code: the policy was never consulted and Battle degenerated into mashing A.
const (
	mainMenuMarker      = "FIGHT"
	moveMenuMarker      = "TYPE/"
	disabledMoveMarker  = "move is disabled"
	useNextMonMarker    = "Use next"
	tryLearnMarker      = "trying to learn"
	abandonLearnMarker  = "Abandon learning"
	trainerSwitchMarker = "change POK"
	forgetMenuMarker    = "forgotten?"
	hmCantDeleteMarker  = "HM techniques"
	switchMenuMarker    = "Choose"
	switchBoxMarker     = "SWITCH"
)

func mainMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, mainMenuMarker)
}

func moveMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, moveMenuMarker) || disabledMoveRefusalUp(m)
}

func disabledMoveRefusalUp(m *emu.Emu) bool {
	return battleScreenHas(m, disabledMoveMarker)
}

func twoOptionPromptUp(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	t := state.ScreenText(&mem)
	return strings.Contains(t, useNextMonMarker) || strings.Contains(t, tryLearnMarker)
}

func twoOptionCursorUp(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeTwoOptionMenu(&mem) != nil
}

func abandonLearnPromptUp(m *emu.Emu) bool {
	return battleScreenHas(m, abandonLearnMarker)
}

func trainerSwitchPromptUp(m *emu.Emu) bool {
	return battleScreenHas(m, trainerSwitchMarker)
}

func forgetMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, forgetMenuMarker)
}

func moveLearnForgetRejected(selectedSlot int, text string) bool {
	return selectedSlot >= 0 && strings.Contains(text, hmCantDeleteMarker)
}

func forgetSlot(romData []byte, moves [4]uint8, tried map[uint8]bool) int {
	damagers := 0
	damages := [4]bool{}
	for i, id := range moves {
		if id == 0 {
			continue
		}
		damages[i] = true
		if mv, err := rom.LookupMove(romData, id); err == nil && mv.Power == 0 {
			damages[i] = false
		}
		if damages[i] {
			damagers++
		}
	}
	for i, id := range moves {
		if id == 0 || tried[id] {
			continue
		}
		if damagers == 1 && damages[i] {
			continue
		}
		return i
	}
	return -1
}

func selectForgetSlot(m *emu.Emu, index int) error {
	var cur int
	stuck := 0
	for i := 0; i < 60; i++ {
		var s state.Mem
		state.Snapshot(m, &s)
		cur = state.DecodeMenu(&s).Current
		if cur == index {
			break
		}
		btn := emu.Down
		if cur > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.CurrentMenuItem)) != cur
		}); err != nil {
			if stuck >= 4 {
				return fmt.Errorf("skill: selectForgetSlot: cursor stuck at %d, wanted %d: %w", cur, index, ErrMenuStuck)
			}
			stuck++
		} else {
			stuck = 0
		}
	}
	var s state.Mem
	state.Snapshot(m, &s)
	if cur = state.DecodeMenu(&s).Current; cur != index {
		return fmt.Errorf("skill: selectForgetSlot: cursor at %d, wanted %d: %w", cur, index, ErrMenuStuck)
	}
	m.Tap(emu.A, 3, 7)
	return nil
}

func battleSwitchMenuUp(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeBattle(&mem) != nil && strings.Contains(state.ScreenText(&mem), switchMenuMarker)
}

func switchBoxUp(m *emu.Emu) bool {
	return battleScreenHas(m, switchBoxMarker)
}

func selectFightEntry(m *emu.Emu) error {
	atFight := func(m *emu.Emu) bool {
		return m.Peek8(sym.TopMenuItemX) == battleMenuLeftX && int(m.Peek8(sym.CurrentMenuItem)) == 0
	}
	for i := 0; i < 8; i++ {
		if atFight(m) {
			return nil
		}
		prevX, prevRow := m.Peek8(sym.TopMenuItemX), int(m.Peek8(sym.CurrentMenuItem))
		var btn emu.Button
		switch {
		case prevX == battleMenuRightX && prevRow != 0:
			btn = emu.Up
		case prevX == battleMenuRightX:
			btn = emu.Left
		default:
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return m.Peek8(sym.TopMenuItemX) != prevX || int(m.Peek8(sym.CurrentMenuItem)) != prevRow
		}); err != nil {
			return fmt.Errorf("skill: Battle: cursor stuck at x=%#02x row %d, want FIGHT (x=%#02x row 0): %w",
				prevX, prevRow, battleMenuLeftX, ErrMenuStuck)
		}
	}
	return fmt.Errorf("skill: Battle: cursor did not reach FIGHT")
}

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
