package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

const (
	vermilionCityMap                uint8 = 0x05
	yellowSquirtleGiftSpecies       uint8 = 0xB1 // SQUIRTLE
	yellowSquirtleJennyGiftX        uint8 = 19
	yellowSquirtleJennyGiftY        uint8 = 15
	yellowSquirtleGiftTravelBattles       = 40
)

func yellowSquirtleGiftDestination() Destination {
	// Approach from the tile south of Officer Jenny's home, matching the
	// Lapras/Dojo pattern: InteractionDestination lets live geometry pick a
	// reachable adjacent stand after Travel enters Vermilion.
	return InteractionDestination(vermilionCityMap, yellowSquirtleJennyGiftX, yellowSquirtleJennyGiftY)
}

func init() {
	interactionPlaces["vermilion officer jenny squirtle"] = yellowSquirtleGiftDestination()
	// Yellow's only guaranteed pre-Cinnabar Surf carrier. Grass never yields a
	// Surf learner, and Lapras is gated behind Saffron (after the Surf-required
	// Cinnabar leg), so roster repair must be able to claim this gift when the
	// Thunder Badge is already owned.
	fieldCarrierGifts = append(fieldCarrierGifts, fieldCarrierGift{
		Species: yellowSquirtleGiftSpecies,
		Ready: func(mem *state.Mem, romData []byte, _ state.StoryFacts) bool {
			return yellowrom.IsCartridge(romData) && state.DecodeProgress(mem).Has(state.BadgeThunder)
		},
		Receive: ReceiveYellowSquirtleGift,
	})
}

// ReceiveYellowSquirtleGift reaches Vermilion City's Officer Jenny and accepts
// her one-time Squirtle through the shared nickname-safe gift driver. The ROM
// asks YES/NO after Thunder Badge before GivePokemon (pokeyellow
// VermilionCity_2.asm), so ConfirmChoice answers that prompt YES and declines
// the later nickname.
func ReceiveYellowSquirtleGift(m *emu.Emu, romData []byte, policy MovePolicy) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: ReceiveYellowSquirtleGift: nil policy")
	}
	if !yellowrom.IsCartridge(romData) {
		return CatchResult{}, fmt.Errorf("skill: ReceiveYellowSquirtleGift: Yellow cartridge required")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if giftPokemonAlreadyOwned(&mem, romData, yellowSquirtleGiftSpecies) {
		return CatchResult{Outcome: OutcomeCaught, Species: yellowSquirtleGiftSpecies}, nil
	}
	if !state.DecodeProgress(&mem).Has(state.BadgeThunder) {
		return CatchResult{}, fmt.Errorf("skill: ReceiveYellowSquirtleGift: Thunder Badge is not owned")
	}

	if _, err := TravelFlee(m, romData, yellowSquirtleGiftDestination(), policy, yellowSquirtleGiftTravelBattles); err != nil {
		return CatchResult{}, fmt.Errorf("skill: ReceiveYellowSquirtleGift: reach Jenny: %w", err)
	}

	return receiveGiftPokemonAt(m, romData, policy, giftPokemonSpec{
		Name:          "ReceiveYellowSquirtleGift",
		Map:           vermilionCityMap,
		X:             yellowSquirtleJennyGiftX,
		Y:             yellowSquirtleJennyGiftY,
		Species:       yellowSquirtleGiftSpecies,
		ConfirmChoice: true,
	})
}
