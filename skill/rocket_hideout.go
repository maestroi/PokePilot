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
	celadonPokemonCenterMap uint8 = 0x85
	gameCornerMap           uint8 = 0x87
	rocketHideoutB1FMap     uint8 = 0xC7
	rocketHideoutB2FMap     uint8 = 0xC8
	rocketHideoutB3FMap     uint8 = 0xC9
	rocketHideoutB4FMap     uint8 = 0xCA

	silphScopeItem uint8 = 0x48

	gameCornerRocketX uint8 = 9
	gameCornerRocketY uint8 = 5
	gameCornerPosterX uint8 = 9
	gameCornerPosterY uint8 = 4
	gameCornerWarpX   uint8 = 17
	gameCornerWarpY   uint8 = 4

	rocketGuard1X uint8 = 23
	rocketGuard1Y uint8 = 12
	rocketGuard2X uint8 = 26
	rocketGuard2Y uint8 = 12
	giovanniX     uint8 = 25
	giovanniY     uint8 = 3
	silphScopeX   uint8 = 25
	silphScopeY   uint8 = 2

	storyBattleSettleBudget = 5000
)

var (
	gameCornerStand = Destination{Map: gameCornerMap, X: 15, Y: 16}
	rocketB3FReturn = Destination{Map: rocketHideoutB3FMap, X: 19, Y: 17}
	rocketB4FEntry  = Destination{Map: rocketHideoutB4FMap, X: 19, Y: 11}
	giovanniStand   = Destination{Map: rocketHideoutB4FMap, X: 25, Y: 4}
)

// RocketHideoutAvailable reports whether the Rocket Hideout story objective
// is actionable from the player's current map. Celadon's Pokemon Center is
// included because a blackout from one of the Rocket fights respawns there;
// the next round must be able to resume the same objective rather than lose
// the story verb exactly when recovery is needed most.
func RocketHideoutAvailable(mapID uint8) bool {
	switch mapID {
	case celadonCityMap, celadonPokemonCenterMap, gameCornerMap,
		rocketHideoutB1FMap, rocketHideoutB2FMap, rocketHideoutB3FMap, rocketHideoutB4FMap:
		return true
	default:
		return false
	}
}

// RocketHideout clears the Celadon Game Corner hideout and collects the
// Silph Scope. Its postcondition is the key item in the bag, not merely a
// Giovanni win: the ROM reveals the Scope as a ground object only after the
// boss script finishes, and Pokemon Tower cannot use a victory flag in its
// place.
//
// The sequence is deliberately resumable. It may start in Celadon, the Game
// Corner, or any hideout floor; already-defeated trainers are harmless and a
// B4F resume from inside the boss room goes straight to Giovanni/the item.
func RocketHideout(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: RocketHideout: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, silphScopeItem); count > 0 {
		return nil
	}

	cur := m.Peek8(sym.CurMap)
	if !RocketHideoutAvailable(cur) {
		return fmt.Errorf("skill: RocketHideout: map %#04x is outside the Celadon/Hideout progression slice", cur)
	}

	// A resume north of B4F's boss door can only happen after that live door
	// has opened. Do not route back through the immutable ROM's closed-door
	// collision just to prove prerequisites that the live map already proved.
	if cur == rocketHideoutB4FMap {
		_, y := playerXY(m)
		if y < 10 {
			return finishRocketBossRoom(m, romData, policy)
		}
	}

	if cur < rocketHideoutB1FMap || cur > rocketHideoutB4FMap {
		if cur != gameCornerMap {
			if _, err := Travel(m, romData, gameCornerStand, policy, 20); err != nil {
				return fmt.Errorf("skill: RocketHideout: reach Game Corner: %w", err)
			}
		}

		// Try the secret stair first. If a previous attempt already pressed
		// the poster switch, this succeeds without replaying the Rocket fight.
		// The route grid is immutable ROM data, so the helper supplies the
		// live-open block only for this crossing.
		if err := crossGameCornerSecretWarp(m, romData); err != nil {
			if err := fightStoryTrainerAt(m, romData, gameCornerRocketX, gameCornerRocketY, "Game Corner Rocket", policy); err != nil {
				return err
			}
			if err := interactHiddenTile(m, romData, gameCornerPosterX, gameCornerPosterY, policy); err != nil {
				return fmt.Errorf("skill: RocketHideout: press switch behind Game Corner poster: %w", err)
			}
			if err := crossGameCornerSecretWarp(m, romData); err != nil {
				return fmt.Errorf("skill: RocketHideout: enter revealed stair: %w", err)
			}
		}
	}

	if m.Peek8(sym.CurMap) != rocketHideoutB4FMap {
		if _, err := Travel(m, romData, rocketB4FEntry, policy, 20); err != nil {
			return fmt.Errorf("skill: RocketHideout: descend to B4F: %w", err)
		}
	}

	// Both guards are south of the boss-room door and can be approached with
	// the ordinary static grid. Once both trainer flags are set, the ROM only
	// replaces the door block on map load, so deliberately leave/re-enter B4F
	// before using the live-open collision override north of it.
	if err := fightStoryTrainerAt(m, romData, rocketGuard1X, rocketGuard1Y, "B4F guard 1", policy); err != nil {
		return err
	}
	if err := fightStoryTrainerAt(m, romData, rocketGuard2X, rocketGuard2Y, "B4F guard 2", policy); err != nil {
		return err
	}
	if _, err := Travel(m, romData, rocketB3FReturn, policy, 20); err != nil {
		return fmt.Errorf("skill: RocketHideout: reload B4F door via B3F: %w", err)
	}
	if _, err := Travel(m, romData, rocketB4FEntry, policy, 20); err != nil {
		return fmt.Errorf("skill: RocketHideout: re-enter B4F after guards: %w", err)
	}

	return finishRocketBossRoom(m, romData, policy)
}

func finishRocketBossRoom(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != rocketHideoutB4FMap {
		return fmt.Errorf("skill: RocketHideout: boss room requested on map %#04x", m.Peek8(sym.CurMap))
	}

	_, y := playerXY(m)
	if y >= 10 {
		if err := walkRocketBossDoor(m, romData); err != nil {
			return fmt.Errorf("skill: RocketHideout: cross B4F boss door: %w", err)
		}
	}
	if err := fightStoryTrainerAt(m, romData, giovanniX, giovanniY, "Giovanni", policy); err != nil {
		return err
	}
	if err := Pickup(m, romData, silphScopeX, silphScopeY, silphScopeItem, policy); err != nil {
		return fmt.Errorf("skill: RocketHideout: collect Silph Scope: %w", err)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, silphScopeItem); count < 1 {
		return fmt.Errorf("skill: RocketHideout: Silph Scope missing from bag after pickup")
	}
	return nil
}

// fightStoryTrainerAt owns one mandatory trainer interaction. If Travel's
// approach already triggered the trainer's sight line, Travel resolves that
// battle before returning. If the object is hidden (the Game Corner Rocket
// or Giovanni after victory), being adjacent with no live sprite proves this
// step was already completed. Otherwise tap the trainer, page only ordinary
// dialogue, and fight if a battle begins. Talking to an already-defeated
// guard simply reaches controllable state with no battle and is a no-op.
func fightStoryTrainerAt(m *emu.Emu, romData []byte, homeX, homeY uint8, name string, policy MovePolicy) error {
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return fmt.Errorf("skill: RocketHideout: parse map %#04x for %s: %w", cur, name, err)
	}
	objectID := 0
	for i, object := range h.Objects {
		if object.X == homeX && object.Y == homeY {
			objectID = i + 1
			break
		}
	}
	if objectID == 0 {
		return fmt.Errorf("skill: RocketHideout: no %s object at (%d,%d) on map %#04x", name, homeX, homeY, cur)
	}

	if err := talkBeside(m, romData, homeX, homeY, policy); err != nil {
		return fmt.Errorf("skill: RocketHideout: approach %s: %w", name, err)
	}
	if m.Peek8(sym.IsInBattle) != 0 {
		return finishStoryBattle(m, name, policy)
	}

	tx, ty, live := liveObjectPosition(m, objectID)
	if !live {
		return nil // hidden by the story script: already defeated
	}
	if err := Face(m, tx, ty); err != nil {
		return fmt.Errorf("skill: RocketHideout: face %s: %w", name, err)
	}

	m.Tap(emu.A, 3, 7)
	var mem state.Mem
	if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return mem.U8(sym.FontLoaded) != 0 || state.DecodeBattle(&mem) != nil
	}); err != nil {
		return fmt.Errorf("skill: RocketHideout: %s interaction opened neither dialogue nor battle", name)
	}

	mem = advanceUntil(m, gymBattleWaitBudget, func(mm *state.Mem) bool {
		return state.DecodeBattle(mm) != nil || (mm.U8(sym.FontLoaded) == 0 && state.Controllable(mm))
	})
	if state.DecodeBattle(&mem) == nil {
		if state.Controllable(&mem) {
			return nil // already-defeated trainer's after-battle text
		}
		return fmt.Errorf("skill: RocketHideout: %s dialogue neither started a battle nor returned control", name)
	}
	return finishStoryBattle(m, name, policy)
}

func finishStoryBattle(m *emu.Emu, name string, policy MovePolicy) error {
	outcome, err := Battle(m, policy)
	if err != nil {
		return fmt.Errorf("skill: RocketHideout: battle %s: %w", name, err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: RocketHideout: %w after losing to %s", ErrBlackedOut, name)
	}

	mem := advanceUntil(m, storyBattleSettleBudget, func(mm *state.Mem) bool {
		return state.Controllable(mm)
	})
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: RocketHideout: not controllable after beating %s", name)
	}
	return nil
}

// replacedBlockCells converts the decomp's ReplaceTileBlock coordinates to
// the four 2x2 game-coordinate cells covered by that map block. The assembly
// loads bc as (blockY, blockX), while world.Grid uses game x/y coordinates.
func replacedBlockCells(blockY, blockX int) [][2]int {
	x, y := blockX*2, blockY*2
	return [][2]int{{x, y}, {x + 1, y}, {x, y + 1}, {x + 1, y + 1}}
}

func applyLiveOpenBlock(g *world.Grid, blockY, blockX int) {
	for _, p := range replacedBlockCells(blockY, blockX) {
		g.Set(p[0], p[1], true)
	}
}

func crossGameCornerSecretWarp(m *emu.Emu, romData []byte) error {
	if m.Peek8(sym.CurMap) != gameCornerMap {
		return fmt.Errorf("on map %#04x, want Game Corner %#04x", m.Peek8(sym.CurMap), gameCornerMap)
	}
	h, err := rom.ParseMap(romData, gameCornerMap)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}
	// GameCornerSetRocketHideoutDoorTile replaces block (y=2,x=8) after
	// EVENT_FOUND_ROCKET_HIDEOUT. The immutable grid still carries $2a.
	applyLiveOpenBlock(grid, 2, 8)

	var push world.Step
	err = walkAround(func() map[[2]int]bool { return spriteBlockers(m) },
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			x, y := playerXY(m)
			steps, p, err := world.FindPathAdjacent(grid, int(x), int(y), int(gameCornerWarpX), int(gameCornerWarpY), blocked)
			if err != nil {
				return nil, err
			}
			push = p
			return steps, nil
		}, func(steps []world.Step) error { return WalkPath(m, steps) },
		func() { m.StepFrames(npcWaitFrames) })
	if err != nil {
		return err
	}

	btn, ok := buttonFor(push)
	if !ok {
		return fmt.Errorf("invalid push %s into Game Corner secret warp", push)
	}
	m.Press(btn)
	crossed := false
	for i := 0; i < crossBudget; i++ {
		if m.Peek8(sym.CurMap) != gameCornerMap {
			crossed = true
			break
		}
		m.StepFrame()
	}
	m.Release(btn)
	if !crossed {
		x, y := playerXY(m)
		return fmt.Errorf("secret stair stayed closed at (%d,%d)", x, y)
	}
	if _, err := m.StepUntil(arriveBudget, func(m *emu.Emu) bool {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return state.Controllable(&mem)
	}); err != nil {
		return fmt.Errorf("player not controllable after secret stair")
	}
	if got := m.Peek8(sym.CurMap); got != rocketHideoutB1FMap {
		return fmt.Errorf("secret stair reached map %#04x, want B1F %#04x", got, rocketHideoutB1FMap)
	}
	return waitForPositionStable(m, positionStableBudget, positionStableFrames)
}

func walkRocketBossDoor(m *emu.Emu, romData []byte) error {
	if m.Peek8(sym.CurMap) != rocketHideoutB4FMap {
		return fmt.Errorf("on map %#04x, want B4F %#04x", m.Peek8(sym.CurMap), rocketHideoutB4FMap)
	}
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}
	// RocketHideoutB4FDoorCallbackScript replaces block (y=5,x=12) with
	// floor once both guards are beaten. Static ROM collision still sees the
	// closed $2d block, so use the live-open shape for this one walk.
	applyLiveOpenBlock(grid, 5, 12)

	return walkAround(func() map[[2]int]bool { return spriteBlockers(m) },
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			x, y := playerXY(m)
			return world.FindPath(grid, int(x), int(y), int(giovanniStand.X), int(giovanniStand.Y), blocked)
		}, func(steps []world.Step) error { return WalkPath(m, steps) },
		func() { m.StepFrames(npcWaitFrames) })
}
