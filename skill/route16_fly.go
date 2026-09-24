package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	route16FlyHouseMap      uint8 = 0xBC
	route16FlyGirlX         uint8 = 2
	route16FlyGirlY         uint8 = 3
	route16FlyHouseStagingX uint8 = 2
	route16FlyHouseStagingY uint8 = 6
	// Celadon-side Route 16 tile east of the Cut tree that joins the lower
	// road to the upper pedestrian passage. Staging here lets GoTo's
	// field-path bridge own that tree before the upper gate hop to the house.
	route16FlyApproachX       uint8 = 30
	route16FlyApproachY       uint8 = 10
	flyPreparationEngagements       = 40
)

const (
	route16FlyHousePlace    = "route 16 fly house"
	route16FlyApproachPlace = "route 16 fly approach"
)

func init() {
	// HM02 is a transaction-owned destination rather than a generic exploration
	// target. Route 16's upper pedestrian passage is deliberately not Bicycle
	// gated; destination-aware local pathing owns the Cut tree on the approach.
	interactionPlaces[route16FlyHousePlace] = Destination{
		Map: route16FlyHouseMap,
		X:   route16FlyHouseStagingX,
		Y:   route16FlyHouseStagingY,
	}
	interactionPlaces[route16FlyApproachPlace] = Destination{
		Map: route16Map,
		X:   route16FlyApproachX,
		Y:   route16FlyApproachY,
	}
}

// PrepareFlyFastTravel owns the complete speed-oriented Fly setup. Success is
// stronger than merely holding HM02: the Thunder Badge must be present and a
// current party member must actually know Fly, which is the exact capability
// Travel's fast-travel chooser requires.
//
// The transaction is checkpoint-safe. A save that already has HM02 skips the
// Route 16 handoff, while a save that already has usable Fly returns
// immediately. Roster/PC/catch policy is deliberately outside this skill: when
// Cut or Fly is not usable it reports a structured field-capability prerequisite
// and lets the objective runtime perform generic repair before retrying.
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
		// Cut along the upper passage returns to Celadon without clearing
		// Snorlax: the tree at (34,9) lands east of him. This guard still
		// refuses the outbound trip until the flute is usable. MEASURED on
		// run-ek112v6wsjd523dbfxfxfk0l0, before that return was routable: a
		// save with HM02 but no Fly carrier and no Poké Flute was stranded in
		// ROUTE_16_FLY_HOUSE and re-failed from map 0xBC on every resume.
		// Refuse the trip up front instead of staking the round trip on a
		// roster repair that may not find a carrier.
		if !redRouteCapabilities(romData, &mem).Has(capCanClearSnorlax) {
			return fmt.Errorf("%w: FLY requires Route 16 Snorlax to be clearable (Poke Flute) before the one-way trip to the Fly house", ErrFieldMovePrerequisite)
		}
		// The secret house is reached through Route 16's upper Cut passage.
		// Declare the semantic requirement; generic prerequisite recovery owns
		// teaching, PC withdrawal, or catch recovery.
		if cut := FieldCapabilityFor(&mem, FieldCut); !cut.Usable {
			return gameruntime.NewFieldCapabilityPrerequisiteMissing("cut")
		}
		if err := EnsureBagSpaceFor(m, fieldHM02Item); err != nil {
			return fmt.Errorf("skill: PrepareFlyFastTravel: make room for HM02: %w", err)
		}
		// Stage onto Route 16's Celadon-side approach first. A direct land plan
		// from Celadon to the Fly house has no honest route until the Route 16
		// Cut tree is cleared; GoTo's field-path bridge owns that tree only
		// while already on Route 16. Skipping this stage used to let a false
		// lower-gate pivot invent a path through the Cycling Road corridor.
		state.Snapshot(m, &mem)
		if cur := state.DecodePlayer(&mem).MapID; cur != route16Map && cur != route16FlyHouseMap {
			approach, ok := Place(route16FlyApproachPlace)
			if !ok {
				return fmt.Errorf("skill: PrepareFlyFastTravel: Route 16 Fly approach destination is not registered")
			}
			if _, err := TravelFlee(m, romData, approach, policy, flyPreparationEngagements); err != nil {
				return fmt.Errorf("skill: PrepareFlyFastTravel: reach Route 16 Fly approach: %w", err)
			}
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

	state.Snapshot(m, &mem)
	fly = FieldCapabilityFor(&mem, FieldFly)
	if !fly.Usable {
		// HM02 acquisition and roster preparation are separate transactions.
		// Returning this typed prerequisite lets generic recovery preserve the
		// other required traversal moves while it makes Fly usable.
		return gameruntime.NewFieldCapabilityPrerequisiteMissing("fly")
	}
	return nil
}
