package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	billsHouseMap      uint8 = 0x58
	vermilionGymMap    uint8 = 0x5C
	ssTicketItem       uint8 = 0x3F
	billPokemonX       uint8 = 6
	billPokemonY       uint8 = 5
	billHumanX         uint8 = 4
	billHumanY         uint8 = 4
	billsPCX           uint8 = 1
	billsPCY           uint8 = 4
	billsPCStandY      uint8 = 5
	vermilionCanCount        = 15
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
