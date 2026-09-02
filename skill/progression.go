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

// finishBillRescue continues the interaction that starts when the player
// talks to Bill in his Pokemon form. The game itself asks for help, walks
// Bill into the separator, and exposes a hidden-event PC at (1,4). Generic
// TalkAt cannot discover that PC because it is not a map object, so this
// story continuation lives beside TalkAt rather than as a fake sprite.
//
// The positive postcondition is the S.S. Ticket in the bag. If the bag is
// full, Bill's script refuses the item and this returns an error rather than
// recording a completed story step.
func finishBillRescue(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != billsHouseMap {
		return fmt.Errorf("skill: Bill: on map %#04x, want Bills House %#04x", m.Peek8(sym.CurMap), billsHouseMap)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, ssTicketItem); count > 0 {
		return nil
	}

	// Bill's first conversation ends by moving him into the machine. Wait
	// for the scripted movement to release control before walking to the PC.
	if !state.Controllable(&mem) {
		if err := Cutscene(m, 4000, func(mm *state.Mem) bool { return true }); err != nil {
			return fmt.Errorf("skill: Bill: enter separator: %w", err)
		}
	}

	// The separator is a hidden background event, not an object. Stand below
	// it, face up, and interact. The PC animation sets the separator event and
	// Bill's map script walks the restored human sprite out of the machine.
	pcStand := Destination{Map: billsHouseMap, X: billsPCX, Y: billsPCStandY}
	if _, err := TravelFlee(m, romData, pcStand, policy, 4); err != nil {
		return fmt.Errorf("skill: Bill: approach cell separator: %w", err)
	}
	if err := Face(m, billsPCX, billsPCY); err != nil {
		return fmt.Errorf("skill: Bill: face cell separator: %w", err)
	}
	if _, err := Talk(m); err != nil {
		return fmt.Errorf("skill: Bill: use cell separator: %w", err)
	}

	// Human Bill's next conversation gives the ticket. TalkAt resolves the
	// live sprite from object 2, so this remains correct after his scripted
	// walk from the machine to the home coordinate at (4,4).
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
//
// It is safe to call when the gate is already open: the first interaction's
// script short-circuits when EVENT_2ND_LOCK_OPENED is set, and Gym will then
// verify reachability by walking to Surge immediately afterward.
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
