package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	viridianCityMap uint8 = 0x01
	viridianGymMap  uint8 = 0x2d

	viridianGymEntranceX uint8 = 16
	viridianGymEntranceY uint8 = 16
	viridianGymLeaderX   uint8 = 2
	viridianGymLeaderY   uint8 = 1

	viridianGymTravelBattles = 60
	viridianGymOpenBudget    = 1200
)

// ViridianGymArrowTilePlayerMovement in pokered/scripts/ViridianGym.asm is
// authoritative for these transitions. Each entry tile starts a simulated
// joypad run; navigation therefore treats the whole forced run as one graph
// edge, exactly like Rocket Hideout's spinner-aware motor.
var viridianGymSpins = map[rocketPoint]rocketPoint{
	{19, 11}: {19, 2},
	{19, 1}:  {11, 1},
	{18, 2}:  {18, 11},
	{11, 2}:  {17, 2},
	{16, 10}: {16, 12},
	{4, 6}:   {4, 13},
	{5, 13}:  {13, 13},
	{4, 14}:  {13, 14},
	{0, 15}:  {0, 7},
	{1, 15}:  {1, 9},
	{13, 16}: {7, 16},
	{13, 17}: {1, 17},
}

var giovanniGym = GymInfo{
	Map:     viridianGymMap,
	Place:   "viridian gym",
	LeaderX: viridianGymLeaderX,
	LeaderY: viridianGymLeaderY,
	Badge:   state.BadgeEarth,
	Leader:  "GIOVANNI",
}

func init() {
	places["viridian gym"] = Destination{Map: viridianGymMap, X: viridianGymLeaderX, Y: viridianGymLeaderY + 1}
	gyms[viridianCityMap] = giovanniGym
	gyms[viridianGymMap] = giovanniGym
}

// ViridianGymReady is the durable story prerequisite. Viridian City sets this
// event only when all seven prior badges are present, so callers never infer an
// open door merely from route knowledge or a previous failed attempt.
func ViridianGymReady(mem *state.Mem) bool {
	inv := state.DecodeInventory(mem)
	return state.DecodeStoryFacts(mem, inv).ViridianGymOpen
}

// EnterViridianGym crosses the real Viridian City warp only after the game's
// own open-gym story fact is visible.
func EnterViridianGym(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) == viridianGymMap {
		return nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !ViridianGymReady(&mem) {
		return fmt.Errorf("skill: EnterViridianGym: Viridian Gym is not open")
	}
	res, err := TravelFlee(m, romData, Destination{Map: viridianGymMap, X: viridianGymEntranceX, Y: viridianGymEntranceY}, policy, viridianGymTravelBattles)
	if err != nil {
		return fmt.Errorf("skill: EnterViridianGym: %w", err)
	}
	if res.BlackedOut {
		return fmt.Errorf("skill: EnterViridianGym: %w", ErrBlackedOut)
	}
	if m.Peek8(sym.CurMap) != viridianGymMap {
		return fmt.Errorf("skill: EnterViridianGym: stopped on map %#04x", m.Peek8(sym.CurMap))
	}
	return nil
}

// walkViridianGymToLeader reuses the tested Rocket Hideout spinner planner,
// but changes the goal from "beside a warp" to "beside Giovanni". Ordinary
// steps and forced arrow entries are planned together from the live position;
// trainer sprites are blockers and trainer battles are surfaced to travel's
// normal battle/replan loop.
func walkViridianGymToLeader(m *emu.Emu, romData []byte) error {
	if m.Peek8(sym.CurMap) != viridianGymMap {
		return fmt.Errorf("skill: ViridianGym: spinner route on map %#04x, want %#04x", m.Peek8(sym.CurMap), viridianGymMap)
	}
	h, err := rom.ParseMap(romData, viridianGymMap)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}

	blocked := spriteBlockers(m)
	sx, sy := playerXY(m)
	actions, err := planRocketSpinner(
		grid.Width, grid.Height, grid.Walkable,
		int(sx), int(sy), int(viridianGymLeaderX), int(viridianGymLeaderY),
		viridianGymSpins, blocked,
	)
	if err != nil {
		return fmt.Errorf("skill: ViridianGym: plan spinner route to Giovanni: %w", err)
	}
	for _, action := range actions {
		if err := executeRocketSpinAction(m, viridianGymMap, action); err != nil {
			return err
		}
	}

	x, y := playerXY(m)
	dx := int(x) - int(viridianGymLeaderX)
	if dx < 0 {
		dx = -dx
	}
	dy := int(y) - int(viridianGymLeaderY)
	if dy < 0 {
		dy = -dy
	}
	if dx+dy != 1 {
		return fmt.Errorf("skill: ViridianGym: spinner route ended at (%d,%d), not beside Giovanni", x, y)
	}
	return nil
}

func travelViridianGymToLeader(m *emu.Emu, romData []byte, policy MovePolicy) (TravelResult, error) {
	return travel(m, policy, viridianGymTravelBattles,
		func() error { return walkViridianGymToLeader(m, romData) },
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fightOnly(m, policy),
	)
}

// ViridianProgression is issue #36's resumable executor. Returning to Viridian
// City is significant: the city script is what commits EVENT_VIRIDIAN_GYM_OPEN
// after seven badges. From there the real door, spinner floor, trainer battles,
// Giovanni battle, and Earth Badge postcondition are all executed from live
// state. A checkpoint already inside the gym resumes directly from its current
// coordinate.
func ViridianProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: ViridianProgression: nil policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	progress := state.DecodeProgress(&mem)
	if progress.Has(state.BadgeEarth) {
		return nil
	}
	if progress.BadgeCount < 7 {
		return fmt.Errorf("skill: ViridianProgression: have %d badges, need seven before Viridian Gym", progress.BadgeCount)
	}

	if m.Peek8(sym.CurMap) != viridianGymMap {
		viridian, ok := Place("viridian city")
		if !ok {
			return fmt.Errorf("skill: ViridianProgression: viridian city place missing")
		}
		res, err := TravelFlee(m, romData, viridian, policy, viridianGymTravelBattles)
		if err != nil {
			return fmt.Errorf("skill: ViridianProgression: return to Viridian City: %w", err)
		}
		if res.BlackedOut {
			return fmt.Errorf("skill: ViridianProgression: %w returning to Viridian City", ErrBlackedOut)
		}

		mem = advanceUntil(m, viridianGymOpenBudget, func(mm *state.Mem) bool {
			return ViridianGymReady(mm) && state.Controllable(mm)
		})
		if !ViridianGymReady(&mem) {
			return fmt.Errorf("skill: ViridianProgression: seven badges did not commit Viridian Gym open after returning to the city")
		}
		if err := EnterViridianGym(m, romData, policy); err != nil {
			return err
		}
	} else if !ViridianGymReady(&mem) {
		return fmt.Errorf("skill: ViridianProgression: inside Viridian Gym without open-gym story fact")
	}

	outcome, err := Gym(m, romData, policy)
	if err != nil {
		return fmt.Errorf("skill: ViridianProgression: %w", err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: ViridianProgression: Giovanni battle outcome %v, want won", outcome)
	}
	state.Snapshot(m, &mem)
	progress = state.DecodeProgress(&mem)
	if !progress.Has(state.BadgeEarth) || progress.BadgeCount != 8 {
		return fmt.Errorf("skill: ViridianProgression: Giovanni win returned with badges=%#02x count=%d, want all eight", progress.Badges, progress.BadgeCount)
	}
	return nil
}
