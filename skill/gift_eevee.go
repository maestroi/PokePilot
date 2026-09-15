package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	celadonMansionRoofHouseMap uint8 = 0x84
	eeveeGiftSpecies           uint8 = 0x66
	eeveeGiftX                 uint8 = 4
	eeveeGiftY                 uint8 = 3
	eeveeGiftApproachX         uint8 = eeveeGiftX - 1
	eeveeGiftApproachY         uint8 = eeveeGiftY
)

func eeveeGiftDestination() Destination {
	// The Eevee ball sits at (4,3). The tile below it, (4,4), is not connected
	// to the room entrance in the live collision grid (the old generic catch
	// pre-travel failed there in #425). Approach laterally from (3,3) instead.
	return Destination{
		Map: celadonMansionRoofHouseMap,
		X:   eeveeGiftApproachX,
		Y:   eeveeGiftApproachY,
	}
}

func init() {
	// Keep the gift itself out of ordinary PlaceNames: travelling to the room
	// without consuming the one-time script is not a useful standalone goal.
	// Dex execution can still resolve this interaction-owned destination.
	interactionPlaces["celadon mansion eevee"] = eeveeGiftDestination()
}

// ReceiveEeveeGift reaches Celadon Mansion's roof house and consumes its
// one-time Eevee Poké Ball gift through the shared nickname-safe,
// ownership-verified GivePokemon driver.
func ReceiveEeveeGift(m *emu.Emu, romData []byte, policy MovePolicy) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: ReceiveEeveeGift: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if giftPokemonAlreadyOwned(&mem, romData, eeveeGiftSpecies) {
		return CatchResult{Outcome: OutcomeCaught, Species: eeveeGiftSpecies}, nil
	}

	// Dex gift objectives deliberately skip agent-level habitat travel. Own the
	// route here so the executor works from any reachable current map rather than
	// assuming the planner already parked Red inside the gift room (#566).
	if _, err := TravelFlee(m, romData, eeveeGiftDestination(), policy, 40); err != nil {
		return CatchResult{}, fmt.Errorf("skill: ReceiveEeveeGift: reach gift: %w", err)
	}

	return receiveGiftPokemonAt(m, romData, policy, giftPokemonSpec{
		Name:    "ReceiveEeveeGift",
		Map:     celadonMansionRoofHouseMap,
		X:       eeveeGiftX,
		Y:       eeveeGiftY,
		Species: eeveeGiftSpecies,
	})
}
