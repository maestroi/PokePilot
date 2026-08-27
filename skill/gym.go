package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// pewterGymMap is the Pewter Gym map: the first gym, and the only one
// the journey reaches before a badge exists.
const pewterGymMap = 0x36

// Brock (sprite 12) stands at (4,1) in the gym's top room;
// Place("pewter gym") is the open floor tile (4,2) directly below him.
const (
	gymLeaderX = 4
	gymLeaderY = 1
)

// gymBattleWaitBudget bounds the leader's intro dialogue to the start of
// the battle: Brock's two text boxes and the battle transition. It is a
// budget, not a prediction; exhausting it is an error with diagnostics.
const gymBattleWaitBudget = 10000

// gymPostBattleBudget bounds the post-victory sequence: the end-of-battle
// text, the "WAIT... TAKE THIS" text, the item-get box, and the TM34
// explanation, all A-advanceable. The badge bit is written by the same
// script that shows those boxes, so the badge appears before the last box
// closes.
const gymPostBattleBudget = 3000

// Gym fights the Pewter Gym leader, Brock. The player must already be on
// the gym map: Gym walks to the approach tile below the leader, faces
// him, opens the trainer dialogue, advances it until the battle starts,
// then fights it to completion with policy.
//
// The postcondition is the badge, not the battle: on a win Gym advances
// the post-victory text until bit 0 of wObtainedBadges is set AND the
// player is Controllable again, so a returned ResultWon means the Boulder
// Badge is provably in RAM. On a loss it advances until controllable
// (the blackout then carries the player to the Pewter center). It is an
// error when the player is not on the gym map, the walk to the leader
// fails, the battle never starts within budget, Battle fails, or the
// post-battle sequence does not finish within budget.
func Gym(m *emu.Emu, romData []byte, policy MovePolicy) (state.BattleResult, error) {
	if policy == nil {
		return 0, fmt.Errorf("skill: Gym: nil policy")
	}
	if cur := m.Peek8(sym.CurMap); cur != pewterGymMap {
		return 0, fmt.Errorf("skill: Gym: player on map %#04x, not the gym %#04x", cur, pewterGymMap)
	}
	dest, ok := Place("pewter gym")
	if !ok {
		return 0, fmt.Errorf("skill: Gym: Place \"pewter gym\" not found")
	}
	if err := walkWithinMap(m, romData, dest); err != nil {
		return 0, fmt.Errorf("skill: Gym: %w", err)
	}
	if err := Face(m, gymLeaderX, gymLeaderY); err != nil {
		return 0, fmt.Errorf("skill: Gym: %w", err)
	}

	// The first A opens the leader's dialogue; advanceUntil keeps tapping
	// A on every box until the battle itself starts (DecodeBattle != nil).
	m.Tap(emu.A, 3, 7)
	mem := advanceUntil(m, gymBattleWaitBudget, func(mm *state.Mem) bool {
		return state.DecodeBattle(mm) != nil
	})
	if state.DecodeBattle(&mem) == nil {
		return 0, fmt.Errorf("skill: Gym: battle did not start after the leader dialogue: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}

	outcome, err := Battle(m, policy)
	if err != nil {
		return outcome, fmt.Errorf("skill: Gym: %w", err)
	}

	// The battle screen is gone, but the post-victory boxes are not yet
	// closed and the badge bit is not yet written. Advance them.
	if outcome == state.ResultWon {
		mem = advanceUntil(m, gymPostBattleBudget, func(mm *state.Mem) bool {
			return mm.U8(sym.ObtainedBadges)&0x01 != 0
		})
		if mem.U8(sym.ObtainedBadges)&0x01 == 0 {
			return outcome, fmt.Errorf("skill: Gym: badge not set %d frames after the victory: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
				gymPostBattleBudget, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
				mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
		}
	}

	// Let the last box close and the font unload so the player is
	// Controllable again (a loss blackouts to the center first).
	mem = advanceUntil(m, gymPostBattleBudget, func(mm *state.Mem) bool {
		return state.Controllable(mm)
	})
	if !state.Controllable(&mem) {
		what := "win"
		if outcome != state.ResultWon {
			what = "lost"
		}
		return outcome, fmt.Errorf("skill: Gym: not controllable %d frames after the %s battle: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			gymPostBattleBudget, what, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	return outcome, nil
}
