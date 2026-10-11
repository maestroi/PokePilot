package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	ssAnneProgressionMaxBattles = 64
	ssAnneCaptainX              = 4
	ssAnneCaptainY              = 2
)

// SSAnneHM01 owns the complete post-Bill transaction that turns the S.S.
// Ticket into HM01/Cut. Generic TravelFlee handles the harbor guard, the 2F
// coordinate-triggered rival battle, and the scripted movement around that
// encounter; this skill owns the durable story postcondition and reward
// preflight rather than asking the strategist to infer the sequence from map
// names.
//
// The operation is resumable. If a checkpoint is already aboard the ship it
// continues from there; if the party needs recovery before the rival it may
// leave for Vermilion's Center because the ship remains present until HM01 is
// obtained. A trainer loss therefore bubbles up through TravelFlee's typed
// ErrTrainerBlackedOut path and the objective can be retried after respawn.
func SSAnneHM01(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: SSAnneHM01: nil battle policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, hm01Item); count > 0 ||
		pcItemQuantity(state.DecodePCItemStorage(&mem).Items, hm01Item) > 0 {
		return nil
	}
	if err := ensureSSTicketInBag(m, romData, &mem, policy); err != nil {
		return err
	}

	// Start the one-rival story leg with a fully recovered party whenever the
	// current checkpoint is not already in that state. Before HM01 the ship
	// cannot have departed, so leaving it temporarily to heal is safe.
	if !allPartyCenterRecovered(&mem) {
		center, ok := Place("vermilion pokemon center")
		if !ok {
			return fmt.Errorf("skill: SSAnneHM01: vermilion pokemon center place missing")
		}
		if _, err := TravelFlee(m, romData, center, policy, ssAnneProgressionMaxBattles); err != nil {
			return fmt.Errorf("skill: SSAnneHM01: travel to Vermilion Pokemon Center: %w", err)
		}
		if err := Heal(m); err != nil {
			return fmt.Errorf("skill: SSAnneHM01: heal before S.S. Anne: %w", err)
		}
	}

	captainRoom, ok := Place("ss anne captain's room")
	if !ok {
		return fmt.Errorf("skill: SSAnneHM01: captain's room place missing")
	}
	if _, err := TravelFlee(m, romData, captainRoom, policy, ssAnneProgressionMaxBattles); err != nil {
		return fmt.Errorf("skill: SSAnneHM01: travel to Captain: %w", err)
	}

	// The Captain's GiveItem branch does not set EVENT_GOT_HM01 if the bag is
	// full. Make room while the player is still controllable, before entering
	// that finite reward conversation.
	if err := EnsureBagSpaceFor(m, hm01Item); err != nil {
		return fmt.Errorf("skill: SSAnneHM01: make room for HM01: %w", err)
	}
	if _, err := TalkAt(m, romData, ssAnneCaptainX, ssAnneCaptainY, policy); err != nil {
		return fmt.Errorf("skill: SSAnneHM01: receive HM01 from Captain: %w", err)
	}

	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, hm01Item); count == 0 {
		return fmt.Errorf("skill: SSAnneHM01: Captain conversation completed without HM01 in the bag (map %#04x at %d,%d)",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
	return nil
}

// ensureSSTicketInBag makes the S.S. Ticket available in the bag before
// boarding. The ticket is a key item the game never consumes, but bag pressure
// (or the retire path after the ship sails) can leave it in the Player PC. The
// dock check reads the bag, so a PC-only ticket cannot board. When the ticket
// is in the PC it is withdrawn; when it is in neither, the boarding
// precondition is genuinely unmet.
func ensureSSTicketInBag(m *emu.Emu, romData []byte, mem *state.Mem, policy MovePolicy) error {
	if _, count := bagEntry(mem, ssTicketItem); count > 0 {
		return nil
	}
	if pcItemQuantity(state.DecodePCItemStorage(mem).Items, ssTicketItem) == 0 {
		return fmt.Errorf("skill: SSAnneHM01: S.S. Ticket is required before boarding")
	}
	if err := EnsureBagSpaceFor(m, ssTicketItem); err != nil {
		return fmt.Errorf("skill: SSAnneHM01: make room for S.S. Ticket: %w", err)
	}
	if err := runInventoryDetour(m, romData, policy, func() error {
		return WithdrawPCItem(m, romData, policy, ssTicketItem, 1)
	}); err != nil {
		return fmt.Errorf("skill: SSAnneHM01: withdraw S.S. Ticket from Player PC: %w", err)
	}
	var after state.Mem
	state.Snapshot(m, &after)
	if _, count := bagEntry(&after, ssTicketItem); count == 0 {
		return fmt.Errorf("skill: SSAnneHM01: S.S. Ticket still not in the bag after withdrawal (map %#04x at %d,%d)",
			after.U8(sym.CurMap), after.U8(sym.XCoord), after.U8(sym.YCoord))
	}
	return nil
}
