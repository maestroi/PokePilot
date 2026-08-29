package skill

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/maestroi/pokepilot/emu"
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

// FirstUsableMove is the default policy: the lowest-numbered move slot
// that has a move and PP remaining. Deterministic, so tests are stable.
// It returns -1 when no slot is usable; Battle checks Usable() before
// calling a policy, so this is only reachable if a policy is called
// directly on an empty-battle state.
func FirstUsableMove(b state.BattleState) int {
	for i, mv := range b.Moves {
		if mv.ID != 0 && mv.PP > 0 {
			return i
		}
	}
	return -1
}

// ErrNoUsableMove reports that every move slot is empty or out of PP.
var ErrNoUsableMove = errors.New("skill: no usable move")

// Frame budgets for Battle. They are upper bounds, not measured timings: a
// real turn (menu + move + resolution) is a few hundred frames and a whole
// battle a few thousand. The total cap exists so a stuck battle fails
// loudly instead of hanging the suite.
const (
	battleFrameCap  = 60000 // total frames for the whole battle
	moveMenuBudget  = 500   // wait for the move menu after selecting FIGHT
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
// A. Battle never uses items, but it does answer the forced switch after a
// faint in a wild battle: the "Use next #MON?" prompt is answered YES
// (NO is an escape attempt) and the first non-fainted party slot is sent
// out. The OTHER half of the party menu — the voluntary switch opened by
// the player through the POKéMON branch — is driven by SwitchActive, not
// here. If the game reaches any other state Battle does not handle, the
// frame cap trips and Battle fails loudly.
//
// Losing is a result, not an error: a blackout returns ResultLost with a
// zbatDebug is read once at init, not per frame: Go 1.27's test harness
// logs every os.Getenv call to the test log (measured: a per-frame getenv
// produced 42MB of "getenv ZBAT" lines in one nine-minute run).
var zbatDebug = os.Getenv("ZBAT") != ""

// nil error. Recovering from a blackout is out of scope.
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

	for {
		if int(m.FrameCount()-startFrame) > battleFrameCap {
			return stuckError(m, fmt.Sprintf("exceeded %d-frame cap", battleFrameCap))
		}

		state.Snapshot(m, &mem)
		if zbatDebug {
			if bs := state.DecodeBattle(&mem); bs != nil {
				fmt.Printf("zbat f=%6d max=%d cur=%d me=%d/%d enemy=%d/%d moves=%v | %s\n",
					m.FrameCount(), m.Peek8(sym.MaxMenuItem), m.Peek8(sym.CurrentMenuItem),
					bs.ActiveHP, bs.ActiveMaxHP, bs.EnemyHP, bs.EnemyMaxHP, bs.Moves,
					strings.Join(strings.Fields(state.ScreenText(&mem)), " "))
			}
		}
		if state.DecodeBattle(&mem) == nil {
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
				x, y := playerXY(m)
				return 0, fmt.Errorf("skill: Battle: map %02x at (%d,%d) battle %+v: %w",
					m.Peek8(sym.CurMap), x, y, bs, ErrNoUsableMove)
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
			// Choose FIGHT. The move menu is picked up on a later pass.
			if err := SelectMenuItem(m, 0); err != nil {
				return menuError(m, "select FIGHT", err)
			}

		case useNextMonUp(m):
			// "Use next #MON?" after the active mon faints in a WILD battle
			// with others alive (core.asm DoUseNextMonDialogue; it returns
			// early for trainer battles). YES proceeds to the party menu; NO
			// is an escape attempt (.tryRunning), which is not what a travel
			// battle wants, so answer YES.
			var s state.Mem
			state.Snapshot(m, &s)
			if state.DecodeTwoOptionMenu(&s) == nil {
				// The prompt text is on screen but the menu cursor is not
				// drawn yet. Step, then look again: a bare continue would skip
				// the StepFrame at the foot of the loop and spin without ever
				// advancing the emulator (measured: frozen frame counter).
				m.StepFrame()
				continue
			}
			// YES is index 0. If this A is lost in the menu's joypad-init
			// window the next pass sees the same prompt and answers again.
			if err := SelectMenuItem(m, 0); err != nil {
				return menuError(m, "answer UseNextMon", err)
			}

		case partyMenuUp(m):
			// The battle party menu (ChooseNextMon). Send out the first slot
			// that is not fainted; the ROM bounces a fainted pick back to the
			// menu, so the choice must come from live party RAM.
			var s state.Mem
			state.Snapshot(m, &s)
			slot := firstLivePartySlot(&s)
			if slot < 0 {
				// No live mon: the ROM would not have opened this menu. Step
				// rather than bare-continue, as in the useNextMon case above.
				m.StepFrame()
				continue
			}
			if err := SelectPartySlot(m, slot); err != nil {
				return menuError(m, "select party slot", err)
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

// useNextMonUp reports whether the "Use next #MON?" prompt after a faint is
// on screen. The yes/no box itself carries no text marker, so this gates on
// the prompt's own line; answering still waits on DecodeTwoOptionMenu seeing
// the drawn cursor (see the case in Battle).
func useNextMonUp(m *emu.Emu) bool {
	return battleScreenHas(m, useNextMonMarker)
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

// firstLivePartySlot returns the index of the first party member that is not
// fainted, or -1 when every member is. The battle party menu bounces a
// fainted pick back to itself (core.asm ChooseNextMon), so a forced switch
// must land on a live slot.
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
