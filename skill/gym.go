package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// GymInfo is what Gym needs to fight one gym: where the leader stands,
// which place name is the tile to stand on, and which bit of wObtainedBadges
// the victory sets. Every field is measured or read from the decomp — none
// of it is derivable at runtime, because "which of this map's trainers is
// the leader" is not a fact the map header states.
type GymInfo struct {
	Map     uint8
	Place   string // the stand-beside tile, from the place table
	LeaderX uint8  // the leader's home tile, faced from Place
	LeaderY uint8
	Badge   state.Badge // the bit of wObtainedBadges a win sets
	Leader  string      // for logs and errors; never parsed
}

// gyms is every gym the journey can currently reach. A gym missing from
// here is a gym Gym refuses to fight rather than one it guesses at: the
// leader tile and the badge bit are the two facts a wrong guess would turn
// into a silent wrong postcondition.
//
// PEWTER_GYM 0x36: Brock at (4,1), Place("pewter gym") is (4,2) below him.
// CERULEAN_GYM 0x41: Misty at (4,2) (data/maps/objects/CeruleanGym.asm),
// one row lower than Brock, so Place("cerulean gym") is (4,3).
// VERMILION_GYM 0x5C: Lt. Surge at (5,1); the stand tile is (5,2), behind
// the two-switch trash-can door. Gym solves that gate before approaching.
var gyms = map[uint8]GymInfo{
	0x36: {Map: 0x36, Place: "pewter gym", LeaderX: 4, LeaderY: 1, Badge: state.BadgeBoulder, Leader: "BROCK"},
	0x41: {Map: 0x41, Place: "cerulean gym", LeaderX: 4, LeaderY: 2, Badge: state.BadgeCascade, Leader: "MISTY"},
	0x5C: {Map: 0x5C, Place: "vermilion gym", LeaderX: 5, LeaderY: 1, Badge: state.BadgeThunder, Leader: "LT. SURGE"},
}

// GymAt reports the gym on a map, if the map is one. Callers use it to ask
// whether a gym challenge is even possible where the player stands.
func GymAt(mapID uint8) (GymInfo, bool) {
	g, ok := gyms[mapID]
	return g, ok
}

// gymBattleWaitBudget bounds the leader's intro dialogue to the start of
// the battle: two or three text boxes and the battle transition. It is a
// budget, not a prediction; exhausting it is an error with diagnostics.
const gymBattleWaitBudget = 10000

// gymPostBattleBudget bounds the post-victory sequence: the end-of-battle
// text, the leader's badge/item boxes and explanation, all A-advanceable.
// The badge bit is written by the same script that shows those boxes, so
// the badge appears before the last box closes.
const gymPostBattleBudget = 3000

// Gym fights the leader of whichever gym the player is standing in (see
// gyms). The player must already be on a gym map: Gym resolves any internal
// approach mechanic it knows (currently Vermilion's trash-can gate), travels
// to the approach tile beside the leader, faces them, opens the trainer
// dialogue, advances it until the battle starts, then fights it to completion
// with policy.
//
// It used to name Brock in its code as well as its comments — the map, the
// leader tile and the badge bit were all Pewter constants — so a second gym
// could not be fought at all, and a Cerulean challenge would have checked
// the Boulder bit and reported a win as a failure.
//
// The postcondition is THAT gym's badge, not the battle: on a win Gym
// advances the post-victory text until the leader's badge bit is set in
// wObtainedBadges AND the player is Controllable again, so a returned
// ResultWon means the badge is provably in RAM. On a loss it advances until
// controllable (the blackout then carries the player to their respawn
// point). It is an error when the player is not on a known gym map, the
// approach fails, the battle never starts within budget, Battle fails, or
// the post-battle sequence does not finish within budget.
func Gym(m *emu.Emu, romData []byte, policy MovePolicy) (state.BattleResult, error) {
	if policy == nil {
		return 0, fmt.Errorf("skill: Gym: nil policy")
	}
	cur := m.Peek8(sym.CurMap)
	g, ok := GymAt(cur)
	if !ok {
		return 0, fmt.Errorf("skill: Gym: map %#04x is not a gym this run can fight", cur)
	}
	dest, ok := Place(g.Place)
	if !ok {
		return 0, fmt.Errorf("skill: Gym: Place %q not found", g.Place)
	}

	// Surge is the first leader whose map itself contains a hard progression
	// gate. Solve the live puzzle first. Its script replaces a collision block
	// in WRAM, so the approach below also needs the live-grid-aware Travel
	// variant instead of rebuilding the original closed door from ROM.
	var (
		res TravelResult
		err error
	)
	if cur == vermilionGymMap {
		if err := OpenVermilionGym(m, romData, policy); err != nil {
			return 0, fmt.Errorf("skill: Gym: open %s's gate: %w", g.Leader, err)
		}
		res, err = travelOpenVermilion(m, romData, dest, policy, 20)
	} else {
		// Travel, not walkWithinMap: a gym's other trainers engage by line of
		// sight on the way to the leader (MEASURED, S7-8: the Pewter cool
		// trainer at (3,6) re-arms on every crossing), and Cerulean's cool
		// trainer at (2,3) faces right along the row the approach tile sits on.
		res, err = Travel(m, romData, dest, policy, 20)
	}
	if err != nil {
		return 0, fmt.Errorf("skill: Gym: approach %s: %w", g.Leader, err)
	}
	if res.BlackedOut {
		return 0, fmt.Errorf("skill: Gym: %w approaching %s (%d battles)", ErrBlackedOut, g.Leader, res.Battles)
	}
	if err := Face(m, g.LeaderX, g.LeaderY); err != nil {
		return 0, fmt.Errorf("skill: Gym: face %s: %w", g.Leader, err)
	}

	m.Tap(emu.A, 3, 7)
	mem := advanceUntil(m, gymBattleWaitBudget, func(mm *state.Mem) bool {
		return state.DecodeBattle(mm) != nil
	})
	if state.DecodeBattle(&mem) == nil {
		return 0, fmt.Errorf("skill: Gym: battle with %s did not start after the leader dialogue: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			g.Leader, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}

	outcome, err := Battle(m, policy)
	if err != nil {
		return outcome, fmt.Errorf("skill: Gym: %w", err)
	}

	if outcome == state.ResultWon {
		mem = advanceUntil(m, gymPostBattleBudget, func(mm *state.Mem) bool {
			return state.DecodeProgress(mm).Has(g.Badge)
		})
		if !state.DecodeProgress(&mem).Has(g.Badge) {
			return outcome, fmt.Errorf("skill: Gym: %s badge not set %d frames after beating %s: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
				g.Badge, gymPostBattleBudget, g.Leader, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
				mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
		}
	}

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
