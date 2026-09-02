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
	billsHouseMap     uint8 = 0x58
	vermilionGymMap   uint8 = 0x5C
	ssTicketItem      uint8 = 0x3F
	billPokemonX      uint8 = 6
	billPokemonY      uint8 = 5
	billHumanX        uint8 = 4
	billHumanY        uint8 = 4
	billsPCX          uint8 = 1
	billsPCY          uint8 = 4
	billsPCStandY     uint8 = 5
	vermilionCanCount       = 15
)

func spriteSlotPresent(mem *state.Mem, slot int) bool {
	for _, sprite := range state.DecodeSprites(mem) {
		if sprite.Slot == slot {
			return true
		}
	}
	return false
}

// helpBill starts the Pokemon-form conversation and deliberately uses the
// cutscene driver instead of Talk. That conversation hands control to a
// scripted walk that lasts longer than Talk's ordinary 40-frame settle
// window, so treating it as an ordinary one-box NPC conversation reports a
// false timeout before Bill reaches the machine.
func helpBill(m *emu.Emu, romData []byte, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, ssTicketItem); count > 0 {
		return nil
	}

	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0
	}); err != nil {
		return fmt.Errorf("skill: Bill: %w", ErrNoDialogue)
	}

	// A is the default YES on Bill's help prompt. Cutscene keeps advancing
	// dialogue and scripted movement until object 1 (Pokemon-form Bill) has
	// been hidden in the separator and control has returned to the player.
	if err := Cutscene(m, 4000, func(mm *state.Mem) bool {
		return !spriteSlotPresent(mm, 1)
	}); err != nil {
		return fmt.Errorf("skill: Bill: enter separator: %w", err)
	}
	return finishBillRescue(m, romData, policy)
}

// finishBillRescue continues after Pokemon-form Bill has entered the Cell
// Separator. The PC is a hidden event at (1,4), so it cannot be surfaced by
// the ordinary map-object talk menu. The positive postcondition is the S.S.
// Ticket in the bag.
func finishBillRescue(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != billsHouseMap {
		return fmt.Errorf("skill: Bill: on map %#04x, want Bills House %#04x", m.Peek8(sym.CurMap), billsHouseMap)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, ssTicketItem); count > 0 {
		return nil
	}

	pcStand := Destination{Map: billsHouseMap, X: billsPCX, Y: billsPCStandY}
	if _, err := TravelFlee(m, romData, pcStand, policy, 4); err != nil {
		return fmt.Errorf("skill: Bill: approach cell separator: %w", err)
	}
	if err := Face(m, billsPCX, billsPCY); err != nil {
		return fmt.Errorf("skill: Bill: face cell separator: %w", err)
	}

	// The hidden PC starts another long scripted handoff: sound effects,
	// Bill's transformation, and his walk out of the separator. Drive that
	// as a cutscene too. Human Bill is object slot 2 and is initially hidden;
	// its appearance is the positive state transition we wait for.
	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(talkOpenBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0
	}); err != nil {
		return fmt.Errorf("skill: Bill: cell separator: %w", ErrNoDialogue)
	}
	if err := Cutscene(m, 6000, func(mm *state.Mem) bool {
		return spriteSlotPresent(mm, 2)
	}); err != nil {
		return fmt.Errorf("skill: Bill: exit separator: %w", err)
	}

	if _, err := TalkAt(m, romData, billHumanX, billHumanY, policy); err != nil {
		return fmt.Errorf("skill: Bill: collect S.S. Ticket: %w", err)
	}
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, ssTicketItem); count == 0 {
		return fmt.Errorf("skill: Bill: conversation finished but S.S. Ticket is not in the bag")
	}
	return nil
}

// vermilionTrashCanCoords maps the hidden-event argument used by the gym
// script to the corresponding trash can tile. The decomp declares the cans
// in column-major order: x=1,3,5,7,9 and y=7,9,11.
func vermilionTrashCanCoords(index uint8) (x, y uint8, ok bool) {
	if index >= vermilionCanCount {
		return 0, 0, false
	}
	return 1 + 2*(index/3), 7 + 2*(index%3), true
}

// interactHiddenTile walks beside a background/hidden-event tile, faces it,
// and advances its text to completion. Unlike TalkAt it deliberately does
// not try to resolve an object slot: trash cans and Bill's PC have none.
func interactHiddenTile(m *emu.Emu, romData []byte, x, y uint8, policy MovePolicy) error {
	if err := talkBeside(m, romData, x, y, policy); err != nil {
		return err
	}
	if err := Face(m, x, y); err != nil {
		return err
	}
	if _, err := Talk(m); err != nil {
		return err
	}
	return nil
}

// OpenVermilionGym solves Lt. Surge's two-switch trash-can gate from the
// game's own live puzzle state. It does not guess or brute-force. The map
// script chooses the first switch in wFirstLockTrashCanIndex; after that can
// succeeds, GymTrashScript writes the generated adjacent second switch into
// wSecondLockTrashCanIndex. Reading those two values means a wrong second
// guess can never reset the puzzle.
func OpenVermilionGym(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != vermilionGymMap {
		return fmt.Errorf("skill: OpenVermilionGym: on map %#04x, want %#04x", m.Peek8(sym.CurMap), vermilionGymMap)
	}
	if policy == nil {
		return fmt.Errorf("skill: OpenVermilionGym: nil policy")
	}

	first := m.Peek8(sym.FirstLockTrashCanIndex)
	x, y, ok := vermilionTrashCanCoords(first)
	if !ok {
		return fmt.Errorf("skill: OpenVermilionGym: first switch index %d is outside 0..14", first)
	}
	if err := interactHiddenTile(m, romData, x, y, policy); err != nil {
		return fmt.Errorf("skill: OpenVermilionGym: first switch %d at (%d,%d): %w", first, x, y, err)
	}

	second := m.Peek8(sym.SecondLockTrashCanIndex)
	x, y, ok = vermilionTrashCanCoords(second)
	if !ok {
		return fmt.Errorf("skill: OpenVermilionGym: second switch index %d is outside 0..14", second)
	}
	if err := interactHiddenTile(m, romData, x, y, policy); err != nil {
		return fmt.Errorf("skill: OpenVermilionGym: second switch %d at (%d,%d): %w", second, x, y, err)
	}
	return nil
}

// travelOpenVermilion is Travel with one live-map correction: opening the
// gym switches replaces map block (2,2) with clear floor in WRAM, while the
// ordinary pathfinder rebuilds collision from the immutable ROM and therefore
// still sees the original double-door block. Only this post-switch approach
// gets the override. Battles and dialogue are resolved by the same Travel
// loop as every other journey.
func travelOpenVermilion(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy, maxBattles int) (TravelResult, error) {
	if maxBattles <= 0 {
		return TravelResult{}, fmt.Errorf("skill: Travel: maxBattles must be > 0, got %d", maxBattles)
	}
	return travel(m, policy, maxBattles,
		func() error { return walkOpenVermilion(m, romData, dest) },
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fightOnly(m, policy),
	)
}

func walkOpenVermilion(m *emu.Emu, romData []byte, dest Destination) error {
	if err := abortIfBattle(m); err != nil {
		return err
	}
	cur := m.Peek8(sym.CurMap)
	if cur != vermilionGymMap || dest.Map != vermilionGymMap {
		return fmt.Errorf("skill: Vermilion Gym live walk requires map %#04x, got current %#04x destination %#04x", vermilionGymMap, cur, dest.Map)
	}
	sx, sy := playerXY(m)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return fmt.Errorf("skill: Vermilion Gym: parse map at (%d,%d): %w", sx, sy, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return fmt.Errorf("skill: Vermilion Gym: build map at (%d,%d): %w", sx, sy, err)
	}

	// VermilionGymSetDoorTile does ReplaceTileBlock with bc=(2,2), replacing
	// that 2x2 game-coordinate block by clear floor block $05. Grid operates
	// in those game coordinates, so the live block covers x=4..5, y=4..5.
	for y := 4; y <= 5; y++ {
		for x := 4; x <= 5; x++ {
			grid.Set(x, y, true)
		}
	}

	var planErr error
	err = walkAround(func() map[[2]int]bool { return spriteBlockers(m) },
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			x, y := playerXY(m)
			steps, err := world.FindPath(grid, int(x), int(y), int(dest.X), int(dest.Y), blocked)
			if err != nil {
				planErr = fmt.Errorf("skill: Vermilion Gym: no live path from (%d,%d) to (%d,%d): %w", x, y, dest.X, dest.Y, err)
				return nil, planErr
			}
			return steps, nil
		}, func(steps []world.Step) error { return WalkPath(m, steps) },
		func() { m.StepFrames(npcWaitFrames) })
	if err == nil || err == planErr {
		return err
	}
	x, y := playerXY(m)
	if errors.Is(err, ErrBattleInterrupted) {
		return fmt.Errorf("skill: Vermilion Gym: battle at (%d,%d): %w", x, y, ErrBattle)
	}
	var eb *ErrBlocked
	if errors.As(err, &eb) {
		return fmt.Errorf("skill: Vermilion Gym: blocked at (%d,%d) after %d retries: %w", eb.At.X, eb.At.Y, maxWalkRetries, err)
	}
	return fmt.Errorf("skill: Vermilion Gym: live walk at (%d,%d): %w", x, y, err)
}
