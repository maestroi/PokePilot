package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	bikeVoucherItem uint8 = 0x2d

	pokemonFanClubMap uint8 = 0x5a
	bikeShopMap       uint8 = 0x42

	fanClubChairmanX uint8 = 3
	fanClubChairmanY uint8 = 1
	fanClubStagingX  uint8 = 2
	fanClubStagingY  uint8 = 6
	bikeShopClerkX   uint8 = 6
	bikeShopClerkY   uint8 = 2

	bicycleTravelMaxBattles = 96
)

const (
	pokemonFanClubChairmanPlace = "pokemon fan club chairman"
	ceruleanBikeShopPlace       = "cerulean bike shop"
)

func init() {
	// These are transaction-owned destinations. They are resolvable by Place
	// for BicycleProgression, but are not generic exploration targets that can
	// stop halfway through the voucher/exchange transaction.
	//
	// Do not route directly to a guessed interaction tile around the chairman.
	// The Fan Club's large table occupies the middle of the room; both (3,2)
	// and (3,3) are inside that blocked footprint. Route only to the open floor
	// just inside the entrance, then let TalkAtChoice/TalkAt find a live
	// reachable side of the chairman using the actual collision grid and sprite
	// positions. This also makes a checkpoint already inside the Fan Club resume
	// straight into the interaction instead of trying to cross furniture first.
	interactionPlaces[pokemonFanClubChairmanPlace] = Destination{Map: pokemonFanClubMap, X: fanClubStagingX, Y: fanClubStagingY}
	interactionPlaces[ceruleanBikeShopPlace] = Destination{Map: bikeShopMap, X: 3, Y: 6}
}

// AcquireBicycle owns Red's complete Bicycle transaction. It is deliberately
// idempotent across checkpoints: a run with the Bike already returns
// immediately, while a run that already collected the Bike Voucher resumes at
// the Cerulean exchange instead of revisiting the Fan Club.
//
// In Pokémon Red the voucher is awarded by the Pokémon Fan Club chairman in
// Vermilion City, then exchanged for the Bicycle at Cerulean's Bike Shop.
func AcquireBicycle(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: AcquireBicycle: nil battle policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if bagHasItem(&mem, bicycleItem) {
		return nil
	}

	if !bagHasItem(&mem, bikeVoucherItem) {
		if err := EnsureBagSpaceFor(m, bikeVoucherItem); err != nil {
			return fmt.Errorf("skill: AcquireBicycle: make room for Bike Voucher: %w", err)
		}
		// If a checkpoint is already inside the Fan Club, do not run another
		// fixed-coordinate GoTo first. The interaction primitive below owns the
		// intra-room approach and can choose a reachable side of the chairman.
		if m.Peek8(sym.CurMap) != pokemonFanClubMap {
			dest, ok := Place(pokemonFanClubChairmanPlace)
			if !ok {
				return fmt.Errorf("skill: AcquireBicycle: Fan Club destination is not registered")
			}
			if _, err := TravelFlee(m, romData, dest, policy, bicycleTravelMaxBattles); err != nil {
				return fmt.Errorf("skill: AcquireBicycle: travel to Pokémon Fan Club: %w", err)
			}
		}

		state.Snapshot(m, &mem)
		before := bagCount(state.DecodeInventory(&mem).Items, bikeVoucherItem)
		if before == 0 {
			if _, err := TalkAtChoice(m, romData, fanClubChairmanX, fanClubChairmanY, 0, policy); err != nil {
				return fmt.Errorf("skill: AcquireBicycle: receive Bike Voucher: %w", err)
			}
		}
		state.Snapshot(m, &mem)
		after := bagCount(state.DecodeInventory(&mem).Items, bikeVoucherItem)
		if after == 0 {
			return fmt.Errorf("skill: AcquireBicycle: Fan Club transaction completed without Bike Voucher (before=%d after=%d)", before, after)
		}
	}

	// BikeShop gives BICYCLE before removing BIKE_VOUCHER, so a full bag still
	// needs one genuinely free slot even though the voucher disappears moments
	// later. Reserve it before starting the exchange script.
	if err := EnsureBagSpaceFor(m, bicycleItem); err != nil {
		return fmt.Errorf("skill: AcquireBicycle: make room for Bicycle: %w", err)
	}
	dest, ok := Place(ceruleanBikeShopPlace)
	if !ok {
		return fmt.Errorf("skill: AcquireBicycle: Bike Shop destination is not registered")
	}
	if _, err := TravelFlee(m, romData, dest, policy, bicycleTravelMaxBattles); err != nil {
		return fmt.Errorf("skill: AcquireBicycle: travel to Cerulean Bike Shop: %w", err)
	}

	state.Snapshot(m, &mem)
	bikeBefore := bagCount(state.DecodeInventory(&mem).Items, bicycleItem)
	voucherBefore := bagCount(state.DecodeInventory(&mem).Items, bikeVoucherItem)
	if bikeBefore > 0 {
		return nil
	}
	if voucherBefore == 0 {
		return fmt.Errorf("skill: AcquireBicycle: reached Bike Shop without Bike Voucher")
	}
	if _, err := TalkAt(m, romData, bikeShopClerkX, bikeShopClerkY, policy); err != nil {
		return fmt.Errorf("skill: AcquireBicycle: exchange Bike Voucher: %w", err)
	}

	state.Snapshot(m, &mem)
	bikeAfter := bagCount(state.DecodeInventory(&mem).Items, bicycleItem)
	voucherAfter := bagCount(state.DecodeInventory(&mem).Items, bikeVoucherItem)
	if bikeAfter == 0 {
		return fmt.Errorf("skill: AcquireBicycle: Bike Shop exchange completed without Bicycle (before=%d after=%d)", bikeBefore, bikeAfter)
	}
	if voucherAfter >= voucherBefore {
		return fmt.Errorf("skill: AcquireBicycle: Bike Voucher was not consumed (before=%d after=%d)", voucherBefore, voucherAfter)
	}
	return nil
}
