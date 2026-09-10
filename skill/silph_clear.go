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

const (
	silphCo3FMap  uint8 = 0xd0
	silphCo7FMap  uint8 = 0xd4
	silphCo11FMap uint8 = 0xeb

	masterBallItemID uint8 = 0x01

	// Ordinary stair landing on 3F. The story route deliberately enters the
	// rival room through 3F's Card Key door and pad instead of relying on the
	// elevator or on a blind floor-by-floor movement script.
	silph3FStairLandingX uint8 = 24
	silph3FStairLandingY uint8 = 1

	// 3F pad (11,11) lands on 7F's pad (5,3), immediately west of the rival
	// encounter. The closed Card Key block is block-coordinate (4,4).
	silph3FTo7FWarpX  uint8 = 11
	silph3FTo7FWarpY  uint8 = 11
	silph3FDoorBlockX       = 4
	silph3FDoorBlockY       = 4

	// The rival appears at home coordinate (3,7) and is triggered from either
	// (3,2) or (3,3). After the fight, the pad at (5,7) lands on 11F (3,2).
	silphRivalHomeX       uint8 = 3
	silphRivalHomeY       uint8 = 7
	silphRivalTriggerX    uint8 = 3
	silphRivalTriggerY    uint8 = 3
	silphRivalAltTriggerX uint8 = 3
	silphRivalAltTriggerY uint8 = 2
	silph7FTo11FWarpX     uint8 = 5
	silph7FTo11FWarpY     uint8 = 7

	// 11F's one Card Key block is block-coordinate (3,6). Its four game
	// cells include Giovanni's two coordinate triggers, (6,13) and (7,12).
	silph11FDoorBlockX                   = 3
	silph11FDoorBlockY                   = 6
	silphGiovanniTriggerX          uint8 = 6
	silphGiovanniTriggerY          uint8 = 13
	silphGiovanniAltTriggerX       uint8 = 7
	silphGiovanniAltTriggerY       uint8 = 12

	silphPresidentX uint8 = 7
	silphPresidentY uint8 = 5

	silphStoryTravelBattles = 160
	silphStoryDriveBudget   = 12000
)

var (
	silph3FTo7FEdge  = world.Edge{Kind: world.EdgeWarp, From: silphCo3FMap, To: silphCo7FMap, WarpX: silph3FTo7FWarpX, WarpY: silph3FTo7FWarpY}
	silph7FTo11FEdge = world.Edge{Kind: world.EdgeWarp, From: silphCo7FMap, To: silphCo11FMap, WarpX: silph7FTo11FWarpX, WarpY: silph7FTo11FWarpY}
)

// ClearSilphCo completes the story portion of issue #34 after the Card Key is
// owned: open only the two doors required by the shortest story route, take
// the 3F pad to the rival, take the 7F pad to 11F, defeat Giovanni, and collect
// the president's Master Ball reward.
//
// Every boundary is reconstructed from RAM. A checkpoint after the rival,
// after Giovanni, or after the president reward resumes from that durable fact
// instead of replaying an earlier fight or declaring the slice complete too
// early.
func ClearSilphCo(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: ClearSilphCo: nil policy")
	}

	facts := currentSilphFacts(m)
	if facts.SilphRescueComplete {
		return nil
	}
	if !facts.SaffronGateOpen {
		return fmt.Errorf("skill: ClearSilphCo: Saffron gate is not open")
	}

	// Giovanni can already be gone in a checkpoint captured between the boss
	// script and the president conversation. Do not route back through the
	// rival room just to prove a battle that RAM already proves complete.
	if facts.SilphCoCleared {
		return collectSilphPresidentReward(m, romData, policy)
	}
	if !facts.CardKeyOwned {
		return fmt.Errorf("skill: ClearSilphCo: Card Key is not owned")
	}

	if !facts.SilphCoRivalDefeated {
		if err := reachSilphRivalRoom(m, romData, policy); err != nil {
			return err
		}
		if err := resolveSilphRival(m, romData, policy); err != nil {
			return err
		}
	}

	// A resume may be anywhere after the rival. Re-enter the exact rival room
	// through the 3F pad when necessary so the 7F->11F pad is reached from the
	// correct connected component rather than by guessing through locked rooms.
	if m.Peek8(sym.CurMap) != silphCo7FMap && m.Peek8(sym.CurMap) != silphCo11FMap {
		if err := reachSilphRivalRoom(m, romData, policy); err != nil {
			return err
		}
	}
	if m.Peek8(sym.CurMap) == silphCo7FMap {
		if err := traverseSilphWarp(m, romData, silph7FTo11FEdge, policy, nil); err != nil {
			return fmt.Errorf("skill: ClearSilphCo: take 7F pad to 11F: %w", err)
		}
	}
	if m.Peek8(sym.CurMap) != silphCo11FMap {
		return fmt.Errorf("skill: ClearSilphCo: story route ended on map %#04x, want Silph Co 11F", m.Peek8(sym.CurMap))
	}

	if err := resolveSilphGiovanni(m, romData, policy); err != nil {
		return err
	}
	return collectSilphPresidentReward(m, romData, policy)
}

func currentSilphFacts(m *emu.Emu) state.StoryFacts {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
}

func reachSilphRivalRoom(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) == silphCo7FMap {
		return nil
	}
	landing := Destination{Map: silphCo3FMap, X: silph3FStairLandingX, Y: silph3FStairLandingY}
	if _, err := TravelFlee(m, romData, landing, policy, silphStoryTravelBattles); err != nil {
		return fmt.Errorf("skill: ClearSilphCo: reach Silph Co 3F: %w", err)
	}
	reachable := func() bool { return silphWarpReachable(m, romData, silph3FTo7FEdge) }
	if !reachable() {
		if err := unlockSilphDoor(m, romData, silph3FDoorBlockX, silph3FDoorBlockY, policy, reachable); err != nil {
			return fmt.Errorf("skill: ClearSilphCo: unlock 3F rival-room door: %w", err)
		}
	}
	if err := traverseSilphWarp(m, romData, silph3FTo7FEdge, policy, nil); err != nil {
		return fmt.Errorf("skill: ClearSilphCo: take 3F pad to 7F: %w", err)
	}
	return nil
}

// unlockSilphDoor interacts with a known Card Key block without assuming which
// of its four 2x2 cells contains the interactive door tile. Each candidate is
// approached through the live grid; A is pressed only while still adjacent,
// and success is the caller's newly-reachable destination, not a press count or
// a particular text page.
func unlockSilphDoor(m *emu.Emu, romData []byte, blockX, blockY int, policy MovePolicy, reachable func() bool) error {
	if reachable() {
		return nil
	}
	var last error
	for _, p := range replacedBlockCells(blockY, blockX) {
		if reachable() {
			return nil
		}
		tx, ty := uint8(p[0]), uint8(p[1])
		dest, move, err := besideDestination(m, romData, tx, ty)
		if err != nil {
			last = err
			continue
		}
		if move {
			if _, err := TravelFlee(m, romData, dest, policy, 40); err != nil {
				return err
			}
		}
		px, py := playerXY(m)
		if _, ok := directionTo(px, py, tx, ty); !ok {
			// Face can move onto a walkable non-door cell. That is evidence this
			// cell is not the closed door; continue from the new live position.
			continue
		}
		if err := Face(m, tx, ty); err != nil {
			last = err
			continue
		}
		px, py = playerXY(m)
		if _, ok := directionTo(px, py, tx, ty); !ok {
			continue
		}

		m.Tap(emu.A, 3, 7)
		if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
			return m.Peek8(sym.FontLoaded) != 0 || m.Peek8(sym.IsInBattle) != 0
		}); err != nil {
			last = err
			continue
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			if err := finishSilphBattle(m, "trainer beside Card Key door", policy); err != nil {
				return err
			}
		}
		if err := settleSilphControl(m, policy); err != nil {
			last = err
			continue
		}
		if reachable() {
			return nil
		}
	}
	if last == nil {
		last = errors.New("no Card Key cell accepted the interaction")
	}
	return fmt.Errorf("door block (%d,%d) did not open a route: %w", blockX, blockY, last)
}

func silphWarpReachable(m *emu.Emu, romData []byte, edge world.Edge) bool {
	if m.Peek8(sym.CurMap) != edge.From {
		return false
	}
	h, err := rom.ParseMap(romData, edge.From)
	if err != nil {
		return false
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return false
	}
	x, y := playerXY(m)
	_, _, _, _, err = warpTarget(h, edge, grid, int(x), int(y), spriteBlockers(m), romData)
	return err == nil
}

func silphTileReachable(m *emu.Emu, romData []byte, mapID, tx, ty uint8) bool {
	if m.Peek8(sym.CurMap) != mapID {
		return false
	}
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return false
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return false
	}
	x, y := playerXY(m)
	_, err = world.FindPath(grid, int(x), int(y), int(tx), int(ty), spriteBlockers(m))
	return err == nil
}

func traverseSilphWarp(m *emu.Emu, romData []byte, edge world.Edge, policy MovePolicy, unlock func() error) error {
	for attempt := 1; attempt <= 6; attempt++ {
		err := Traverse(m, romData, edge)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrBattle) {
			if battleErr := finishSilphBattle(m, "Silph trainer interruption", policy); battleErr != nil {
				return battleErr
			}
			if settleErr := settleSilphControl(m, policy); settleErr != nil {
				return settleErr
			}
			continue
		}
		if errors.Is(err, ErrLegUnwalkable) && unlock != nil {
			if unlockErr := unlock(); unlockErr != nil {
				return unlockErr
			}
			continue
		}
		return err
	}
	return fmt.Errorf("warp %02x(%d,%d)->%02x did not complete after retries", edge.From, edge.WarpX, edge.WarpY, edge.To)
}

func resolveSilphRival(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if currentSilphFacts(m).SilphCoRivalDefeated {
		return nil
	}
	if m.Peek8(sym.CurMap) != silphCo7FMap {
		return fmt.Errorf("skill: ClearSilphCo: rival resolution requested on map %#04x", m.Peek8(sym.CurMap))
	}

	for _, trigger := range []Destination{
		{Map: silphCo7FMap, X: silphRivalTriggerX, Y: silphRivalTriggerY},
		{Map: silphCo7FMap, X: silphRivalAltTriggerX, Y: silphRivalAltTriggerY},
	} {
		_, err := TravelFlee(m, romData, trigger, policy, 40)
		if err != nil && !silphStoryActive(m) {
			return fmt.Errorf("skill: ClearSilphCo: reach rival trigger (%d,%d): %w", trigger.X, trigger.Y, err)
		}
		if driveErr := driveSilphStory(m, "Silph rival", policy, func(f state.StoryFacts) bool {
			return f.SilphCoRivalDefeated
		}); driveErr == nil {
			return nil
		} else if currentSilphFacts(m).SilphCoRivalDefeated {
			return nil
		}
	}
	return fmt.Errorf("skill: ClearSilphCo: rival triggers completed without silph rival defeated fact")
}

func resolveSilphGiovanni(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if currentSilphFacts(m).SilphCoCleared {
		return nil
	}
	if m.Peek8(sym.CurMap) != silphCo11FMap {
		return fmt.Errorf("skill: ClearSilphCo: Giovanni resolution requested on map %#04x", m.Peek8(sym.CurMap))
	}

	reachable := func() bool {
		return silphTileReachable(m, romData, silphCo11FMap, silphGiovanniTriggerX, silphGiovanniTriggerY)
	}
	if !reachable() {
		if err := unlockSilphDoor(m, romData, silph11FDoorBlockX, silph11FDoorBlockY, policy, reachable); err != nil {
			return fmt.Errorf("skill: ClearSilphCo: unlock 11F boss-room door: %w", err)
		}
	}

	for _, trigger := range []Destination{
		{Map: silphCo11FMap, X: silphGiovanniTriggerX, Y: silphGiovanniTriggerY},
		{Map: silphCo11FMap, X: silphGiovanniAltTriggerX, Y: silphGiovanniAltTriggerY},
	} {
		_, err := TravelFlee(m, romData, trigger, policy, 60)
		if err != nil && !silphStoryActive(m) {
			return fmt.Errorf("skill: ClearSilphCo: reach Giovanni trigger (%d,%d): %w", trigger.X, trigger.Y, err)
		}
		if driveErr := driveSilphStory(m, "Silph Giovanni", policy, func(f state.StoryFacts) bool {
			return f.SilphCoCleared
		}); driveErr == nil {
			return nil
		} else if currentSilphFacts(m).SilphCoCleared {
			return nil
		}
	}
	return fmt.Errorf("skill: ClearSilphCo: Giovanni triggers completed without silph_co_cleared")
}

func silphStoryActive(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeBattle(&mem) != nil || mem.U8(sym.FontLoaded) != 0 || !state.Controllable(&mem)
}

func driveSilphStory(m *emu.Emu, name string, policy MovePolicy, done func(state.StoryFacts) bool) error {
	var mem state.Mem
	for spent := 0; spent < silphStoryDriveBudget; spent += 10 {
		state.Snapshot(m, &mem)
		facts := state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
		if done(facts) && state.Controllable(&mem) {
			return nil
		}
		if state.DecodeBattle(&mem) != nil {
			if err := finishSilphBattle(m, name, policy); err != nil {
				return err
			}
			continue
		}
		switch interaction := state.DecodeInteraction(&mem); interaction.Kind {
		case state.InteractionDialogue:
			m.Tap(emu.A, 3, 7)
		case state.InteractionNone:
			m.StepFrames(10)
		default:
			return fmt.Errorf("skill: ClearSilphCo: unexpected %s interaction %q during %s", interaction.Kind, interaction.Text, name)
		}
	}
	return fmt.Errorf("skill: ClearSilphCo: %s story script exceeded %d frames", name, silphStoryDriveBudget)
}

func finishSilphBattle(m *emu.Emu, name string, policy MovePolicy) error {
	outcome, err := Battle(m, policy)
	if err != nil {
		return fmt.Errorf("skill: ClearSilphCo: battle %s: %w", name, err)
	}
	if outcome != state.ResultWon {
		return fmt.Errorf("skill: ClearSilphCo: %w after losing %s", ErrTrainerBlackedOut, name)
	}
	return nil
}

func settleSilphControl(m *emu.Emu, policy MovePolicy) error {
	var mem state.Mem
	for spent := 0; spent < storyBattleSettleBudget; spent += 10 {
		state.Snapshot(m, &mem)
		if state.Controllable(&mem) {
			return nil
		}
		if state.DecodeBattle(&mem) != nil {
			if err := finishSilphBattle(m, "trainer interruption", policy); err != nil {
				return err
			}
			continue
		}
		switch interaction := state.DecodeInteraction(&mem); interaction.Kind {
		case state.InteractionDialogue:
			m.Tap(emu.A, 3, 7)
		case state.InteractionNone:
			m.StepFrames(10)
		default:
			return fmt.Errorf("skill: ClearSilphCo: unexpected interaction %q while settling", interaction.Kind)
		}
	}
	return fmt.Errorf("skill: ClearSilphCo: control did not return within %d frames", storyBattleSettleBudget)
}

func collectSilphPresidentReward(m *emu.Emu, romData []byte, policy MovePolicy) error {
	facts := currentSilphFacts(m)
	if facts.SilphRescueComplete {
		return nil
	}
	if !facts.SilphCoCleared {
		return fmt.Errorf("skill: ClearSilphCo: president reward requested before Giovanni is defeated")
	}
	if facts.MasterBallAwarded {
		return nil
	}

	if err := EnsureBagSpaceFor(m, masterBallItemID); err != nil {
		return fmt.Errorf("skill: ClearSilphCo: make room for Master Ball: %w", err)
	}
	if m.Peek8(sym.CurMap) != silphCo11FMap {
		dest := Destination{Map: silphCo11FMap, X: silphPresidentX, Y: silphPresidentY + 1}
		if _, err := TravelFlee(m, romData, dest, policy, silphStoryTravelBattles); err != nil {
			return fmt.Errorf("skill: ClearSilphCo: return to Silph president: %w", err)
		}
	}
	if _, err := TalkAt(m, romData, silphPresidentX, silphPresidentY, policy); err != nil {
		return fmt.Errorf("skill: ClearSilphCo: receive president reward: %w", err)
	}
	facts = currentSilphFacts(m)
	if !facts.MasterBallAwarded || !facts.SilphRescueComplete {
		return fmt.Errorf("skill: ClearSilphCo: president conversation ended without Master Ball award completion")
	}
	return nil
}
