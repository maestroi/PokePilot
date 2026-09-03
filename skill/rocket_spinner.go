package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const rocketSpinSettleBudget = 2400

type rocketPoint struct {
	x, y int
}

type rocketSpinAction struct {
	Step    world.Step
	Enter   rocketPoint
	Landing rocketPoint
	Forced  bool
}

// These are the final coordinates selected by the Rocket Hideout arrow-tile
// scripts. A planned input enters the key tile and the game owns movement
// until Landing; navigation must therefore model the whole forced transition
// as one state edge instead of pretending it was an ordinary one-tile step.
var rocketB2FSpins = map[rocketPoint]rocketPoint{
	{4, 9}: {2, 9}, {4, 11}: {8, 11}, {4, 15}: {8, 11}, {4, 16}: {8, 11},
	{4, 19}: {2, 19}, {4, 22}: {2, 19}, {5, 14}: {9, 16}, {6, 22}: {6, 20},
	{6, 24}: {6, 20}, {8, 9}: {2, 9}, {8, 12}: {8, 11}, {8, 15}: {8, 11},
	{8, 19}: {2, 19}, {8, 23}: {2, 19}, {9, 14}: {9, 16}, {9, 22}: {9, 24},
	{10, 9}: {2, 9}, {10, 10}: {2, 9}, {10, 15}: {2, 9}, {10, 17}: {14, 15},
	{10, 19}: {14, 15}, {10, 25}: {14, 25}, {11, 14}: {15, 18}, {11, 16}: {15, 18},
	{11, 18}: {11, 20}, {12, 9}: {2, 9}, {12, 11}: {2, 9}, {12, 13}: {2, 9},
	{12, 17}: {14, 15}, {13, 10}: {14, 12}, {13, 12}: {14, 12}, {13, 16}: {15, 18},
	{13, 18}: {11, 20}, {13, 19}: {14, 15}, {13, 22}: {9, 24}, {13, 23}: {2, 19},
	{14, 17}: {14, 15}, {15, 16}: {15, 18}, {16, 14}: {16, 13}, {16, 16}: {16, 13},
	{16, 18}: {16, 13}, {17, 10}: {14, 12}, {17, 11}: {2, 9},
}

var rocketB3FSpins = map[rocketPoint]rocketPoint{
	{10, 13}: {14, 13}, {10, 19}: {18, 15}, {11, 18}: {15, 22}, {12, 11}: {10, 11},
	{12, 17}: {18, 15}, {12, 20}: {18, 15}, {13, 16}: {17, 16}, {14, 11}: {16, 11},
	{14, 15}: {18, 15}, {14, 17}: {18, 15}, {14, 19}: {18, 15}, {15, 16}: {17, 16},
	{15, 18}: {15, 22}, {16, 13}: {16, 11}, {17, 12}: {17, 16}, {18, 16}: {18, 15},
}

func rocketSpinnerTransitions(mapID uint8) map[rocketPoint]rocketPoint {
	switch mapID {
	case rocketHideoutB2FMap:
		return rocketB2FSpins
	case rocketHideoutB3FMap:
		return rocketB3FSpins
	default:
		return nil
	}
}

// planRocketSpinner treats each direction press as one graph edge. Entering a
// spinner key jumps the resulting state to the script's landing coordinate;
// ordinary cells still advance exactly one tile. The goal is a walkable tile
// adjacent to the exit warp so Traverse can own the final warp step.
func planRocketSpinner(width, height int, walkable func(int, int) bool, sx, sy, warpX, warpY int, transitions map[rocketPoint]rocketPoint, blocked map[[2]int]bool) ([]rocketSpinAction, error) {
	start := rocketPoint{sx, sy}
	if sx < 0 || sy < 0 || sx >= width || sy >= height || !walkable(sx, sy) {
		return nil, fmt.Errorf("skill: RocketHideout: spinner start (%d,%d) is not walkable", sx, sy)
	}
	adjacent := func(p rocketPoint) bool {
		dx := p.x - warpX
		if dx < 0 {
			dx = -dx
		}
		dy := p.y - warpY
		if dy < 0 {
			dy = -dy
		}
		return dx+dy == 1
	}
	if adjacent(start) {
		return nil, nil
	}

	steps := []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight}
	queue := []rocketPoint{start}
	seen := map[rocketPoint]bool{start: true}
	type previous struct {
		from   rocketPoint
		action rocketSpinAction
	}
	prev := make(map[rocketPoint]previous)
	var goal rocketPoint
	found := false

	for len(queue) > 0 && !found {
		cur := queue[0]
		queue = queue[1:]
		for _, step := range steps {
			enter := rocketPoint{cur.x + step.DX, cur.y + step.DY}
			if enter.x < 0 || enter.y < 0 || enter.x >= width || enter.y >= height || !walkable(enter.x, enter.y) || blocked[[2]int{enter.x, enter.y}] {
				continue
			}
			landing := enter
			forced := false
			if p, ok := transitions[enter]; ok {
				landing = p
				forced = true
				if landing.x < 0 || landing.y < 0 || landing.x >= width || landing.y >= height || !walkable(landing.x, landing.y) || blocked[[2]int{landing.x, landing.y}] {
					continue
				}
			}
			if seen[landing] {
				continue
			}
			seen[landing] = true
			prev[landing] = previous{from: cur, action: rocketSpinAction{Step: step, Enter: enter, Landing: landing, Forced: forced}}
			if adjacent(landing) {
				goal = landing
				found = true
				break
			}
			queue = append(queue, landing)
		}
	}
	if !found {
		return nil, fmt.Errorf("skill: RocketHideout: no spinner-aware path from (%d,%d) beside warp (%d,%d): %w", sx, sy, warpX, warpY, world.ErrNoPath)
	}

	rev := make([]rocketSpinAction, 0, len(prev))
	for at := goal; at != start; {
		p := prev[at]
		rev = append(rev, p.action)
		at = p.from
	}
	out := make([]rocketSpinAction, len(rev))
	for i := range rev {
		out[len(rev)-1-i] = rev[i]
	}
	return out, nil
}

func executeRocketSpinAction(m *emu.Emu, mapID uint8, action rocketSpinAction) error {
	if !action.Forced {
		err := WalkPath(m, []world.Step{action.Step})
		if errors.Is(err, ErrBattleInterrupted) {
			x, y := playerXY(m)
			return fmt.Errorf("skill: RocketHideout: battle during spinner-floor walk at (%d,%d): %w", x, y, ErrBattle)
		}
		return err
	}

	startX, startY := playerXY(m)
	if int(startX)+action.Step.DX != action.Enter.x || int(startY)+action.Step.DY != action.Enter.y {
		return fmt.Errorf("skill: RocketHideout: stale spinner action from (%d,%d), expected entry (%d,%d)", startX, startY, action.Enter.x, action.Enter.y)
	}
	btn, ok := buttonFor(action.Step)
	if !ok {
		return fmt.Errorf("skill: RocketHideout: invalid spinner step %s", action.Step)
	}

	m.Press(btn)
	moved := false
	for i := 0; i < stepMoveBudget; i++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil {
			m.Release(btn)
			return fmt.Errorf("skill: RocketHideout: battle entering spinner at (%d,%d): %w", action.Enter.x, action.Enter.y, ErrBattle)
		}
		if state.DecodeDialogue(&mem) != nil {
			m.Release(btn)
			return ErrDialogueInterrupted
		}
		x, y := playerXY(m)
		if x != startX || y != startY {
			moved = true
			break
		}
		m.StepFrame()
	}
	m.Release(btn)
	if !moved {
		return &ErrBlocked{Step: action.Step, At: struct{ X, Y uint8 }{startX, startY}}
	}

	for i := 0; i < rocketSpinSettleBudget; i++ {
		if m.Peek8(sym.CurMap) != mapID {
			return fmt.Errorf("skill: RocketHideout: spinner unexpectedly left map %#04x for %#04x", mapID, m.Peek8(sym.CurMap))
		}
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil {
			return fmt.Errorf("skill: RocketHideout: battle during forced spinner movement: %w", ErrBattle)
		}
		if state.DecodeDialogue(&mem) != nil {
			return ErrDialogueInterrupted
		}
		x, y := playerXY(m)
		if int(x) == action.Landing.x && int(y) == action.Landing.y && state.Controllable(&mem) && mem.U8(sym.WalkCounter) == 0 {
			return waitForPositionStable(m, positionStableBudget, positionStableFrames)
		}
		m.StepFrame()
	}
	x, y := playerXY(m)
	return fmt.Errorf("skill: RocketHideout: spinner from (%d,%d) did not settle at (%d,%d); ended at (%d,%d)", action.Enter.x, action.Enter.y, action.Landing.x, action.Landing.y, x, y)
}

func walkRocketSpinnerWarp(m *emu.Emu, romData []byte, mapID, toMap uint8, warpX, warpY uint8) error {
	if m.Peek8(sym.CurMap) != mapID {
		return fmt.Errorf("skill: RocketHideout: spinner warp requested on map %#04x, want %#04x", m.Peek8(sym.CurMap), mapID)
	}
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}
	transitions := rocketSpinnerTransitions(mapID)
	if len(transitions) == 0 {
		return fmt.Errorf("skill: RocketHideout: map %#04x has no spinner transition table", mapID)
	}

	for attempt := 0; ; attempt++ {
		blocked := spriteBlockers(m)
		x, y := playerXY(m)
		actions, planErr := planRocketSpinner(grid.Width, grid.Height, grid.Walkable, int(x), int(y), int(warpX), int(warpY), transitions, blocked)
		if planErr != nil {
			if len(blocked) == 0 || attempt >= maxWalkRetries {
				return planErr
			}
			m.StepFrames(npcWaitFrames)
			continue
		}

		replan := false
		for _, action := range actions {
			if err := executeRocketSpinAction(m, mapID, action); err != nil {
				var eb *ErrBlocked
				if errors.As(err, &eb) && attempt < maxWalkRetries {
					m.StepFrames(npcWaitFrames)
					replan = true
					break
				}
				return err
			}
		}
		if replan {
			continue
		}
		return Traverse(m, romData, world.Edge{Kind: world.EdgeWarp, From: mapID, To: toMap, WarpX: warpX, WarpY: warpY})
	}
}

func travelRocketWarp(m *emu.Emu, policy MovePolicy, goTo func() error) error {
	_, err := travel(m, policy, 20,
		goTo,
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fightOnly(m, policy),
	)
	return err
}

// descendRocketHideout uses the ordinary B1F stair, then models B2F/B3F's
// forced arrow tiles explicitly. It is safe to resume on any of those floors:
// every iteration starts from the live map/coordinate and only advances one
// floor before re-reading RAM.
func descendRocketHideout(m *emu.Emu, romData []byte, policy MovePolicy) error {
	for {
		switch m.Peek8(sym.CurMap) {
		case rocketHideoutB1FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: rocketHideoutB2FMap, WarpX: 23, WarpY: 2}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("skill: RocketHideout: B1F -> B2F: %w", err)
			}
		case rocketHideoutB2FMap:
			if err := travelRocketWarp(m, policy, func() error {
				return walkRocketSpinnerWarp(m, romData, rocketHideoutB2FMap, rocketHideoutB3FMap, 21, 8)
			}); err != nil {
				return fmt.Errorf("skill: RocketHideout: cross B2F spinner floor: %w", err)
			}
		case rocketHideoutB3FMap:
			if err := travelRocketWarp(m, policy, func() error {
				return walkRocketSpinnerWarp(m, romData, rocketHideoutB3FMap, rocketHideoutB4FMap, 19, 18)
			}); err != nil {
				return fmt.Errorf("skill: RocketHideout: cross B3F spinner floor: %w", err)
			}
		case rocketHideoutB4FMap:
			return nil
		default:
			return fmt.Errorf("skill: RocketHideout: cannot descend from map %#04x", m.Peek8(sym.CurMap))
		}
	}
}
