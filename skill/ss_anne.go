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
	if _, count := bagEntry(&mem, hm01Item); count > 0 {
		return nil
	}
	if _, count := bagEntry(&mem, ssTicketItem); count == 0 {
		return fmt.Errorf("skill: SSAnneHM01: S.S. Ticket is required before boarding")
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
