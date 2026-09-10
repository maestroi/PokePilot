package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const billRouteMaxBattles = 64

// BillProgressionAvailable bounds the owned Bill story objective to the
// Cerulean/Nugget Bridge slice where it is useful. In particular it remains
// available immediately after Misty (Cerulean Gym) and after a blackout
// respawns the player at the Cerulean Pokemon Center.
func BillProgressionAvailable(mapID uint8) bool {
	switch mapID {
	case 0x03, // CERULEAN_CITY
		0x23, // ROUTE_24
		0x24, // ROUTE_25
		0x3e, // CERULEAN_TRASHED_HOUSE
		0x40, // CERULEAN_POKECENTER
		0x41, // CERULEAN_GYM
		billsHouseMap:
		return true
	default:
		return false
	}
}

// Bill rescues Bill and collects the S.S. Ticket. The objective owns the
// complete transaction rather than asking the planner to infer a sequence of
// travel/talk/hidden-PC actions from place names.
//
// TravelFlee preserves the normal journey contract on the way north: wild
// encounters are fled and mandatory trainers are fought. A trainer loss is
// therefore surfaced as the ordinary typed blackout error; after respawn the
// objective remains available and can be retried from Cerulean.
func Bill(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: Bill: nil battle policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, ssTicketItem); count > 0 {
		return nil
	}

	if m.Peek8(sym.CurMap) != billsHouseMap {
		dest, ok := Place("bill's house")
		if !ok {
			return fmt.Errorf("skill: Bill: Place %q not found", "bill's house")
		}
		if _, err := TravelFlee(m, romData, dest, policy, billRouteMaxBattles); err != nil {
			return fmt.Errorf("skill: Bill: travel to Bill's house: %w", err)
		}
	}

	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, ssTicketItem); count > 0 {
		return nil
	}
	// Bill's final human-form conversation awards a new distinct key item.
	// Reserve the slot before entering either half of the scripted rescue so
	// a full bag cannot finish the cutscene yet fail the progression reward.
	if err := EnsureBagSpaceFor(m, ssTicketItem); err != nil {
		return fmt.Errorf("skill: Bill: make room for S.S. Ticket: %w", err)
	}

	// Pokemon-form Bill is object slot 1. Approach him explicitly before
	// helpBill: that helper intentionally owns the unusual choice + scripted
	// walk and must not be driven through ordinary TalkAt settling.
	if spriteSlotPresent(&mem, 1) {
		if err := talkBeside(m, romData, billPokemonX, billPokemonY, policy); err != nil {
			return fmt.Errorf("skill: Bill: approach Bill: %w", err)
		}
		if err := Face(m, billPokemonX, billPokemonY); err != nil {
			return fmt.Errorf("skill: Bill: face Bill: %w", err)
		}
		if err := helpBill(m, romData, policy); err != nil {
			return err
		}
	} else if err := finishBillRescue(m, romData, policy); err != nil {
		// A resumed checkpoint can already be between Bill entering the
		// separator and receiving the ticket. finishBillRescue is idempotent
		// over that half-complete state.
		return err
	}

	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, ssTicketItem); count == 0 {
		return fmt.Errorf("skill: Bill: completed without S.S. Ticket in the bag")
	}
	return nil
}
