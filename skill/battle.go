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
	// the ROM bounced as permanent. pendingLearn* is the positive postcondition:
	// the offered move must appear in that exact party slot before Battle exits.
	var lastForgetSlot = -1
	var triedForgets map[uint8]bool
	pendingLearnMove := uint8(0)
	pendingLearnSlot := -1
	pendingLearnPartySlot := -1
	forcedChoiceVisits := 0
	itemUses := 0
	voluntarySwitches := 0

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
			// The battle ended. Settle any end-of-battle text and wait
			// until the player is controllable, then report the result.
			if err := settleAfterBattle(m, &mem); err != nil {
				return 0, err
			}
			state.Snapshot(m, &mem)
			return state.DecodeBattleResult(&mem), nil
		}

		// One decision per iteration, and every branch re-reads the screen
		// next time round. Waiting inside a branch for the next menu to
		// appear is what made this brittle: a single missed transition
		// turned into a hard error mid-fight instead of another look.
		switch {
		case moveMenuUp(m):
			bs := state.DecodeBattle(&mem)
			if bs == nil {
				continue // the battle ended while the menu was up
			}
			usable := bs.Usable()
			if len(usable) == 0 {
				// Normally the main-menu preflight below catches this before
				// FIGHT opens a move menu. If the ROM nevertheless presents one,
				// back out and either switch to PP or let AnyMoveToSelect choose
				// STRUGGLE on the next FIGHT attempt. Never select an exhausted
				// or disabled slot just to make the menu go away.
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
			// The move menu is 1-indexed: MoveSelectionMenu stores
			// wPlayerMoveListIndex+1 into wCurrentMenuItem, so slot i sits
			// at cursor i+1.
			if err := SelectMenuItem(m, slot+1); err != nil {
				return menuError(m, "select move", err)
			}
			// Give the menu a chance to go away. If it has not, the next
			// iteration simply looks again rather than failing.
			_, _ = m.StepUntil(moveCloseBudget, func(m *emu.Emu) bool {
				return !moveMenuUp(m)
			})

		case mainMenuUp(m):
			// PP recovery comes first: if this active mon cannot legally attack
			// and a live bench mon can, switching is the bounded escape from an
			// otherwise pointless turn. If no such bench exists we deliberately
			// continue to FIGHT; pret/pokered's AnyMoveToSelect selects STRUGGLE
			// when all moves are 0 PP and/or disabled.
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

			// A tactical switch is considered before spending medicine. The
			// shared combat scorer compares every healthy bench member against
			// the current opponent; a switch must clear a material-gain threshold
			// and the per-battle cap, so equivalent parties cannot ping-pong.
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

			// Automatic medicine is intentionally conservative and bounded.
			// A successful use returns as soon as RAM proves both effect and
			// consumption; the enemy response is then resumed by this state
			// machine on the next pass.
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

			// Choose FIGHT. The move menu is picked up on a later pass.
			//
			// FOUND live (checkpointed repro of the item-19 wedge, cross-
			// checked against pret/pokered's DisplayBattleMenu): this is a
			// 2x2 grid — column via wTopMenuItemX, row via wCurrentMenuItem
			// (0 or mainMenuMax) — and Up/Down never cross columns
			// (.leftColumn_WaitForInput/.rightColumn_WaitForInput each
			// watch only one of Left/Right, plus A). SelectMenuItem's
			// generic Up/Down-only stepper silently does nothing when the
			// cursor is on PKMN or RUN (the right column), which is exactly
			// where it lands after backing out of the forced switch menu
			// (wBattleAndStartSavedMenuItem restored to PKMN's slot). Every
			// other caller reaches this case fresh, cursor already on
			// FIGHT, so the gap was never exercised before. selectFightEntry
			// is the same grid-walk skill/party.go's SwitchActive already
			// uses for PKMN and skill/flee.go's selectRunEntry uses for RUN.
			if err := selectFightEntry(m); err != nil {
				return menuError(m, "select FIGHT", err)
			}
			if err := SelectMenuItem(m, 0); err != nil {
				return menuError(m, "select FIGHT", err)
			}

		case forgetMenuUp(m):
			// "Which move should be forgotten?" follows a deliberate YES on
			// TryingToLearn. Re-evaluate from live RAM so an HM rejection can
			// remove that move from consideration without guessing a cursor path.
			bs := state.DecodeBattle(&mem)
			if bs == nil {
				continue // the battle ended while the menu was up
			}
			if lastForgetSlot >= 0 {
				// The menu reappeared: the ROM rejected the last pick as an HM
				// technique (or another permanent move), so remember the fact.
				if triedForgets == nil {
					triedForgets = map[uint8]bool{}
				}
				triedForgets[bs.Moves[lastForgetSlot].ID] = true
				lastForgetSlot = -1
			}
			ids := [4]uint8{bs.Moves[0].ID, bs.Moves[1].ID, bs.Moves[2].ID, bs.Moves[3].ID}
			offered := m.Peek8(sym.MoveNum)
			decision := decideNaturalMove(m.ROM(), bs.ActiveType1, bs.ActiveType2, ids, offered, triedForgets)
			if !decision.Learn || decision.ReplaceSlot < 0 {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d): accepted natural move %d but no legal strategic replacement remains: %s",
					m.Peek8(sym.CurMap), x, y, offered, decision.Reason)
			}
			if zbatDebug {
				fmt.Printf("zbat move-learn action=replace %s\n", decision.Reason)
			}
			slot := decision.ReplaceSlot
			pendingLearnMove = offered
			pendingLearnSlot = slot
			pendingLearnPartySlot = int(m.Peek8(sym.PlayerMonNumber))
			if err := selectForgetSlot(m, slot); err != nil {
				return menuError(m, "select move to forget", err)
			}
			lastForgetSlot = slot

		case trainerSwitchPromptUp(m):
			// After a trainer's mon faints, shift battle style asks whether
			// to change Pokémon before the replacement is sent out
			// (core.asm EnemySendOut). NO keeps the healthy active mon and
			// continues the battle. A blind A chooses YES, opens the forced
			// "Bring out which Pokémon?" menu, and selecting the first live
			// slot then bounces forever when that slot is already out.
			var s state.Mem
			state.Snapshot(m, &s)
			if state.DecodeTwoOptionMenu(&s) == nil {
				// The marker is on the final line immediately before the yes/no
				// box. Let the ROM finish drawing it without another A: that A
				// can land after the cursor appears and accidentally choose YES.
				m.StepFrame()
				continue
			}
			if err := selectTwoOption(m, 1); err != nil {
				return menuError(m, "decline trainer switch", err)
			}

		case abandonLearnPromptUp(m):
			// A strategic NO on TryingToLearn leads to a second confirmation:
			// "Abandon learning <MOVE>?". YES (index 0) is the intentional
			// completion of that decision; do not leave it to blind A-mashing.
			var s state.Mem
			state.Snapshot(m, &s)
			if state.DecodeTwoOptionMenu(&s) == nil {
				m.StepFrame()
				continue
			}
			if err := SelectMenuItem(m, 0); err != nil {
				return menuError(m, "confirm decline of natural move", err)
			}

		case twoOptionPromptUp(m):
			// "Use next #MON?" is always YES. A natural move prompt is now a
			// scored decision: YES only if the resulting four-move set improves;
			// otherwise NO proceeds to the explicit abandon confirmation above.
			var s state.Mem
			state.Snapshot(m, &s)
			text := state.ScreenText(&s)
			if state.DecodeTwoOptionMenu(&s) == nil {
				// The try-learn text has a <CONT> before the menu. A is required
				// to reveal the offered move and draw the cursor; UseNextMon has
				// no such wait, where the tap is harmless.
				m.Tap(emu.A, 3, 7)
				continue
			}
			choice := 0 // YES for Use next #MON?
			if strings.Contains(text, tryLearnMarker) {
				bs := state.DecodeBattle(&s)
				if bs == nil {
					continue
				}
				ids := [4]uint8{bs.Moves[0].ID, bs.Moves[1].ID, bs.Moves[2].ID, bs.Moves[3].ID}
				offered := m.Peek8(sym.MoveNum)
				decision := decideNaturalMove(m.ROM(), bs.ActiveType1, bs.ActiveType2, ids, offered, nil)
				if !decision.Learn {
					choice = 1 // NO: then confirm Abandon learning on the next prompt
				}
				if zbatDebug {
					action := "learn"
					if !decision.Learn {
						action = "decline"
					}
					fmt.Printf("zbat move-learn action=%s offered=%d reason=%s\n", action, offered, decision.Reason)
				}
			}
			if err := SelectMenuItem(m, choice); err != nil {
				return menuError(m, "answer two-option prompt", err)
			}

		case partyMenuUp(m):
			// The battle party menu (ChooseNextMon). Rank every live member
			// against the current opponent and send out the best replacement;
			// if battle state vanished mid-menu, preserve the old first-live
			// legality fallback.
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
				// No live mon: the ROM would not have opened this menu. Step
				// rather than bare-continue, as in the useNextMon case above.
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
			// SWITCH/STATS/CANCEL box (engine/battle/core.asm
			// .partyMonWasSelected — CONFIRMED against pret/pokered): B is
			// watched here and answers "Cancel selected"'s sibling,
			// .partyMonDeselected — back to the party list, NOT out of the
			// battle. forcedChoiceVisits (checked in battleSwitchMenuUp
			// below, which every round-trip through this machinery passes
			// through) bounds the whole cycle, in case some other screen
			// this project hasn't seen yet also matches switchBoxMarker.
			m.Tap(emu.B, 3, 7)

		case battleSwitchMenuUp(m):
			// forcedChoiceVisits bounds the WHOLE forced-choice cycle, not
			// just the sub-box: MEASURED live, once B correctly exits back
			// to the main menu (below), selecting FIGHT from a cursor left
			// on RUN can land back here instead of the move menu — the main
			// menu is a 2x2 grid (mainMenuMax) and the generic
			// SelectMenuItem's linear Up/Down assumption does not hold for
			// it, a latent bug this is the first path ever to exercise
			// (every other caller reaches the main menu with the cursor
			// already on FIGHT). That is a separate bug; this bound keeps
			// it from turning into another silent 60000-frame spin while it
			// is unfixed.
			forcedChoiceVisits++
			if forcedChoiceVisits > forcedChoiceCap {
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d): %w", m.Peek8(sym.CurMap), x, y, ErrForcedChoiceStuck)
			}
			// The VOLUNTARY switch party list — Battle never opens this
			// itself (mainMenuUp normally picks FIGHT), so reaching it means a
			// caller left the battle mid-transition: Flee does exactly that
			// after a trainer refuses RUN (ErrTrainerBattle's doc comment on
			// wForcePlayerToChooseMon).
			//
			// CONFIRMED against pret/pokered (home/pokemon.asm
			// PartyMenuInit, engine/battle/core.asm .partyMonDeselected):
			// the FIRST time this list is up, wForcePlayerToChooseMon has
			// disabled B — only A (a slot pick) is watched, which is what
			// opens the SWITCH/STATS/CANCEL box above. Backing out of that
			// box calls GoBackToPartyMenu, which re-runs PartyMenuInit; the
			// flag is already cleared by then, so B is watched THIS time —
			// pressing it now genuinely exits the party menu back to the
			// main battle menu (.checkIfPartyMonWasSelected takes the
			// carry-set path to .quitPartyMenu). So: pick the slot on the
			// first visit, press B on every visit after.
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
			// Text or an animation. Advance it and look again.
			m.Tap(emu.A, 3, 7)
		}
	}
}

// Battle menus are identified by what the game has drawn into wTileMap,
// because wFontLoaded — which every overworld skill relies on — is MEASURED
// to stay 0 for the whole of a battle. Battle text does not go through the
// overworld text engine. Gating on it made this whole state machine dead
// code: the policy was never consulted and Battle degenerated into mashing A.
//
// wTileMap is RAM, not the framebuffer, so reading it is not screen-scraping;
// it is the same source the dialogue tracer already decodes.
//
// wMaxMenuItem alone cannot do this job: it holds the move menu's value
// (numMoves+1) while the "used TACKLE!" text that follows is on screen.
const (
	mainMenuMarker   = "FIGHT"    // only on the FIGHT/ITEM/PKMN/RUN menu
	moveMenuMarker   = "TYPE/"    // only on the move-selection menu
	useNextMonMarker = "Use next" // only on UseNextMonText (data/text/text_2.asm:889)
	// tryLearnMarker is on the "<NAME> is trying to learn <MOVE>" prompt that
	// GainExperience prints when a level-up offers a move while all four slots
	// are full (learn_move.asm TryingToLearnText). ScreenText joins the box's
	// lines with single spaces and text wraps at word boundaries, so the
	// phrase stays contiguous no matter where the line breaks.
	tryLearnMarker = "trying to learn"
	// abandonLearnMarker is the confirmation shown after answering NO to
	// TryingToLearn. YES there intentionally completes the strategic decline.
	abandonLearnMarker = "Abandon learning"
	// trainerSwitchMarker is on TrainerAboutToUseText, the shift-style
	// prompt shown between a trainer's party members. Battle answers NO so
	// the current healthy mon stays out; YES opens BATTLE_PARTY_MENU.
	trainerSwitchMarker = "change POK"
	// forgetMenuMarker is on "Which move should be forgotten?", the move list
	// printed after answering YES to the try-learn prompt.
	forgetMenuMarker = "forgotten?"
	// switchMenuMarker is the NORMAL_PARTY_MENU footer ("Choose a #MON."),
	// which the VOLUNTARY mid-battle switch prints: core.asm .partyMenuWasSelected
	// sets wPartyMenuTypeOrMessageID to NORMAL_PARTY_MENU, unlike the forced
	// switch's BATTLE_PARTY_MENU ("Bring out", partyMenuMarker). It comes from
	// wTileMap like every other battle marker and is only meaningful while a
	// battle is in progress — the overworld party screen prints the same line.
	switchMenuMarker = "Choose"
	// switchBoxMarker is on the SWITCH/STATS/CANCEL box
	// (SWITCH_STATS_CANCEL_MENU_TEMPLATE) that follows a slot pick in the
	// voluntary party menu; the forced switch has no such box.
	switchBoxMarker = "SWITCH"
)

// mainMenuUp reports whether the FIGHT/ITEM/PKMN/RUN menu is up.
func mainMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, mainMenuMarker)
}

// moveMenuUp reports whether the move-selection menu is up.
func moveMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, moveMenuMarker)
}

// twoOptionPromptUp reports whether a yes/no prompt Battle must answer on
// purpose is on screen: the "Use next #MON?" after a faint, or "<NAME> is
// trying to learn <MOVE>" when a level-up offers a move while all four slots
// are full. Both render the same TWO_OPTION_MENU; the marker only decides
// WHICH prompt it is — whether one is actually up is DecodeTwoOptionMenu's
// job in the case (it checks the drawn cursor).
func twoOptionPromptUp(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	t := state.ScreenText(&mem)
	return strings.Contains(t, useNextMonMarker) || strings.Contains(t, tryLearnMarker)
}

func abandonLearnPromptUp(m *emu.Emu) bool {
	return battleScreenHas(m, abandonLearnMarker)
}

// trainerSwitchPromptUp reports whether the trainer replacement prompt is
// being drawn or its yes/no box is waiting for input.
func trainerSwitchPromptUp(m *emu.Emu) bool {
	return battleScreenHas(m, trainerSwitchMarker)
}

// forgetMenuUp reports whether the "Which move should be forgotten?" move
// list is on screen — the menu that follows a YES on the try-learn prompt.
func forgetMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, forgetMenuMarker)
}

// forgetSlot is the conservative legacy replacement helper used by the Cut
// teaching path. Natural level-up learning no longer uses slot order: it calls
// DecideNaturalMove/decideNaturalMove and can decline the offered move.
func forgetSlot(romData []byte, moves [4]uint8, tried map[uint8]bool) int {
	damagers := 0
	damages := [4]bool{}
	for i, id := range moves {
		if id == 0 {
			continue
		}
		damages[i] = true // assume damaging until the table says otherwise
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
			continue // the only damaging option stays
		}
		return i
	}
	return -1
}

// selectForgetSlot moves the cursor of the "Which move should be forgotten?"
// list to index and presses A, step-and-verify: each direction tap is
// followed by a re-read of wCurrentMenuItem, and A is pressed only once the
// cursor reads index. The cursor index is the positive fact, never a press
// count.
//
// It cannot use SelectMenuItem: that helper treats wMaxMenuItem as an
// exclusive count, but this menu stores wNumMovesMinusOne (3 for four
// moves), so its last slot would be rejected as out of range.
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

// battleSwitchMenuUp reports whether the VOLUNTARY battle party menu is on
// screen: a party menu drawn while a battle is in progress. The forced
// switch after a faint prints the BATTLE_PARTY_MENU footer ("Bring out"),
// which partyMenuUp matches; this one prints the NORMAL_PARTY_MENU footer
// ("Choose a #MON."), and no other battle screen contains it.
func battleSwitchMenuUp(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeBattle(&mem) != nil && strings.Contains(state.ScreenText(&mem), switchMenuMarker)
}

// switchBoxUp reports whether the SWITCH/STATS/CANCEL box is on screen.
func switchBoxUp(m *emu.Emu) bool {
	return battleScreenHas(m, switchBoxMarker)
}

// selectFightEntry moves the FIGHT/ITEM/PKMN/RUN cursor to FIGHT — left
// column, row 0. The grid is 2 columns by mainMenuMax+1 rows with
// wMaxMenuItem == 1 per column (comment on mainMenuMax), so a plain
// SelectMenuItem cannot reach it from the right column (PKMN/RUN): Up/Down
// never cross columns (engine/battle/core.asm .leftColumn_WaitForInput /
// .rightColumn_WaitForInput each watch only one of Left/Right, plus A).
// Same shape as skill/party.go's SwitchActive (targets PKMN) and
// skill/flee.go's selectRunEntry (targets RUN) — every tap is verified
// against wTopMenuItemX and wCurrentMenuItem before the next one, never a
// press count.
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
			btn = emu.Up // RUN -> PKMN
		case prevX == battleMenuRightX:
			btn = emu.Left // PKMN -> FIGHT: LEFT keeps the row
		default:
			btn = emu.Up // left column below the target: back up
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

// firstLivePartySlot returns the index of the first party member that is not
// fainted, or -1 when every member is. The battle party menu bounces a
// fainted pick back to itself (core.asm ChooseNextMon), so a forced switch
// must land on a live slot. This remains the legality fallback for menu states
// where no current opponent can be decoded for tactical replacement scoring.
func firstLivePartySlot(mem *state.Mem) int {
	party := state.DecodeParty(mem)
	for i, mon := range party.Mons {
		if !mon.Fainted() {
			return i
		}
	}
	return -1
}

// battleScreenHas reports whether marker appears in the text the game has
// drawn into wTileMap.
func battleScreenHas(m *emu.Emu, marker string) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return strings.Contains(state.ScreenText(&mem), marker)
}

// settleAfterBattle advances any end-of-battle text boxes and waits until
// the player is controllable again. It returns an error if the player is
// still not controllable after the budget.
func settleAfterBattle(m *emu.Emu, mem *state.Mem) error {
	startFrame := m.FrameCount()
	for int(m.FrameCount()-startFrame) < settleBudget {
		state.Snapshot(m, mem)
		if state.Controllable(mem) {
			return nil
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

// stuckError builds a diagnosable error for a stuck battle, carrying the
// map, coordinates, and decoded battle state.
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

// menuError wraps a SelectMenuItem failure with the battle context.
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

// containsInt reports whether slice contains x.
func containsInt(slice []int, x int) bool {
	for _, v := range slice {
		if v == x {
			return true
		}
	}
	return false
}
