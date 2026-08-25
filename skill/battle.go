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
// A. Battle never uses items or switches mons; if the game reaches a state
// it does not handle (e.g. the party menu after a faint), the frame cap
// trips and Battle fails loudly.
//
// Losing is a result, not an error: a blackout returns ResultLost with a
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
		if os.Getenv("ZBAT") != "" {
			if bs := state.DecodeBattle(&mem); bs != nil {
				fmt.Printf("zbat f=%6d max=%d cur=%d me=%d/%d enemy=%d/%d moves=%v | %s\n",
					m.FrameCount(), m.Peek8(sym.MaxMenuItem), m.Peek8(sym.CurrentMenuItem),
					bs.ActiveHP, bs.ActiveMaxHP, bs.EnemyHP, bs.EnemyMaxHP, bs.Moves,
					strings.Join(strings.Fields(state.ScreenText(&mem)), " "))
			}
		}
		if state.DecodeBattle(&mem) == nil {
			if os.Getenv("ZBAT") != "" {
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
	mainMenuMarker = "FIGHT" // only on the FIGHT/ITEM/PKMN/RUN menu
	moveMenuMarker = "TYPE/" // only on the move-selection menu
)

// mainMenuUp reports whether the FIGHT/ITEM/PKMN/RUN menu is up.
func mainMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, mainMenuMarker)
}

// moveMenuUp reports whether the move-selection menu is up.
func moveMenuUp(m *emu.Emu) bool {
	return battleScreenHas(m, moveMenuMarker)
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
