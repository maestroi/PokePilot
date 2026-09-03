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

var surgeGym = GymInfo{Map: 0x5C, Place: "vermilion gym", LeaderX: 5, LeaderY: 1, Badge: state.BadgeThunder, Leader: "LT. SURGE"}

// gyms is every gym the journey can currently challenge. Vermilion City is
// deliberately an alias for Lt. Surge: unlike Brock and Misty, the third gym
// has a field-move prerequisite outside its map. Offering the challenge from
// the city lets Gym own that prerequisite atomically — find/cut the tree,
// enter the gym, solve the trash switches, then fight the leader — instead of
// requiring the planner to invent an unexpressed "use Cut" objective.
var gyms = map[uint8]GymInfo{
	0x36: {Map: 0x36, Place: "pewter gym", LeaderX: 4, LeaderY: 1, Badge: state.BadgeBoulder, Leader: "BROCK"},
	0x41: {Map: 0x41, Place: "cerulean gym", LeaderX: 4, LeaderY: 2, Badge: state.BadgeCascade, Leader: "MISTY"},
	0x05: surgeGym, // Vermilion City: exterior Cut prerequisite
	0x5C: surgeGym, // Vermilion Gym: already inside
	0x9D: {Map: 0x9D, Place: "fuchsia gym", LeaderX: 4, LeaderY: 10, Badge: state.BadgeSoul, Leader: "KOGA"},
}

// GymAt reports the gym challenge available from a map, if there is one.
func GymAt(mapID uint8) (GymInfo, bool) {
	g, ok := gyms[mapID]
	return g, ok
}

const gymBattleWaitBudget = 10000
const gymPostBattleBudget = 3000

// Gym executes the complete challenge available where the player stands.
// Brock, Misty, and Koga begin inside their gyms. Surge may begin in
// Vermilion City: Gym then owns the exterior Cut prerequisite, the internal
// trash-can gate, the leader battle, and the Thunder Badge postcondition.
func Gym(m *emu.Emu, romData []byte, policy MovePolicy) (state.BattleResult, error) {
	if policy == nil {
		return 0, fmt.Errorf("skill: Gym: nil policy")
	}
	cur := m.Peek8(sym.CurMap)
	g, ok := GymAt(cur)
	if !ok {
		return 0, fmt.Errorf("skill: Gym: map %#04x has no gym challenge this run can execute", cur)
	}
	dest, ok := Place(g.Place)
	if !ok {
		return 0, fmt.Errorf("skill: Gym: Place %q not found", g.Place)
	}

	if cur == vermilionCity {
		if err := EnterVermilionGym(m, romData, policy); err != nil {
			return 0, fmt.Errorf("skill: Gym: reach %s: %w", g.Leader, err)
		}
		cur = m.Peek8(sym.CurMap)
	}

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
		// Travel, not walkWithinMap: gym trainers can engage by line of sight
		// on the way to the leader, and Travel resolves those battles before
		// replanning from the resulting live position. Fuchsia's invisible
		// walls are encoded in the map collision itself, so ordinary pathing
		// follows the legal maze without a blind input script.
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
