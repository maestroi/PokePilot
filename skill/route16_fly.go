package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	route16FlyHouseMap       uint8 = 0xBC
	route16FlyGirlX          uint8 = 2
	route16FlyGirlY          uint8 = 3
	route16FlyHouseStagingX  uint8 = 2
	route16FlyHouseStagingY  uint8 = 6
	flyPreparationEngagements      = 40
)

const route16FlyHousePlace = "route 16 fly house"

func init() {
	// HM02 is a transaction-owned destination rather than a generic exploration
	// target. Route 16's upper pedestrian passage is deliberately not Bicycle
	// gated; destination-aware local pathing owns the Cut tree on the approach.
	interactionPlaces[route16FlyHousePlace] = Destination{
		Map: route16FlyHouseMap,
		X:   route16FlyHouseStagingX,
		Y:   route16FlyHouseStagingY,
	}
}

// PrepareFlyFastTravel owns the complete speed-oriented Fly setup. Success is
// stronger than merely holding HM02: the Thunder Badge must be present and a
// current party member must actually know Fly, which is the exact capability
// Travel's fast-travel chooser requires.
//
// The transaction is checkpoint-safe. A save that already has HM02 skips the
// Route 16 handoff, while a save that already has usable Fly returns
// immediately. When the current roster cannot learn Fly, the shared field
// roster repair may withdraw or catch a compatible Pokemon while preserving
// every already-unlocked core traversal move.
func PrepareFlyFastTravel(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: PrepareFlyFastTravel: nil move policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	fly := FieldCapabilityFor(&mem, FieldFly)
	if fly.Usable {
		return nil
	}
	if !fly.BadgeOwned {
		return fmt.Errorf("%w: FLY requires the %s Badge", ErrFieldMovePrerequisite, fly.Badge)
	}

	if !fly.HMOwned {
		// The secret house is reached through Route 16's upper Cut passage.
		// Repair Cut before committing to the detour so a resumed run whose
		// carrier changed does not reach Celadon and then fail at the tree.
		if err := RepairUtilityFieldCapability(m, romData, policy, FieldCut); err != nil {
			return fmt.Errorf("skill: PrepareFlyFastTravel: prepare Cut for Route 16: %w", err)
		}
		if err := EnsureBagSpaceFor(m, fieldHM02Item); err != nil {
			return fmt.Errorf("skill: PrepareFlyFastTravel: make room for HM02: %w", err)
		}
		dest, ok := Place(route16FlyHousePlace)
		if !ok {
			return fmt.Errorf("skill: PrepareFlyFastTravel: Route 16 Fly house destination is not registered")
		}
		if _, err := TravelFlee(m, romData, dest, policy, flyPreparationEngagements); err != nil {
			return fmt.Errorf("skill: PrepareFlyFastTravel: reach Route 16 Fly house: %w", err)
		}

		state.Snapshot(m, &mem)
		before := bagCount(state.DecodeInventory(&mem).Items, fieldHM02Item)
		if before == 0 {
			if _, err := TalkAt(m, romData, route16FlyGirlX, route16FlyGirlY, policy); err != nil {
				return fmt.Errorf("skill: PrepareFlyFastTravel: receive HM02: %w", err)
			}
		}
		state.Snapshot(m, &mem)
		after := bagCount(state.DecodeInventory(&mem).Items, fieldHM02Item)
		if after == 0 {
			return fmt.Errorf("skill: PrepareFlyFastTravel: Route 16 handoff completed without HM02 (before=%d after=%d)", before, after)
		}
	}

	// Preserve every mandatory traversal capability the save has already
	// unlocked while adding Fly. At the normal Celadon point this is Cut+Fly;
	// resumed/later saves may also need Surf or Strength retained.
	state.Snapshot(m, &mem)
	required := append(OwnedCoreProgressionFieldMoves(&mem), FieldFly)
	err := RepairFieldCapabilities(m, romData, policy, required)
	if errors.Is(err, ErrFieldRosterNoBalls) {
		// A run with no compatible party/box member may need one nearby wild
		// carrier. Stock the minimum progression reserve and retry the same
		// deterministic roster repair instead of abandoning Fly permanently.
		if _, stockErr := EnsureProgressionPokeBalls(m, romData, policy); stockErr != nil {
			return fmt.Errorf("skill: PrepareFlyFastTravel: stock Poke Balls for Fly carrier: %w", stockErr)
		}
		err = RepairFieldCapabilities(m, romData, policy, required)
	}
	if err != nil {
		return fmt.Errorf("skill: PrepareFlyFastTravel: prepare Fly carrier: %w", err)
	}

	state.Snapshot(m, &mem)
	fly = FieldCapabilityFor(&mem, FieldFly)
	if !fly.Usable {
		return fmt.Errorf("skill: PrepareFlyFastTravel: final Fly invariant failed: badge=%v HM=%v learned=%v slot=%d",
			fly.BadgeOwned, fly.HMOwned, fly.Learned, fly.PartySlot)
	}
	return nil
}
