package skill

import "github.com/maestroi/pokepilot/emu"

const (
	celadonMansionRoofHouseMap uint8 = 0x84
	eeveeGiftSpecies           uint8 = 0x66
	eeveeGiftX                 uint8 = 4
	eeveeGiftY                 uint8 = 3
)

func init() {
	// Keep the gift itself out of ordinary PlaceNames: travelling to the room
	// without consuming the one-time script is not a useful standalone goal.
	// Dex execution can still resolve this interaction-owned destination.
	interactionPlaces["celadon mansion eevee"] = Destination{
		Map: celadonMansionRoofHouseMap,
		X:   eeveeGiftX,
		Y:   eeveeGiftY + 1,
	}
}

// ReceiveEeveeGift consumes Celadon Mansion's one-time Eevee Poké Ball gift
// through the shared nickname-safe, ownership-verified GivePokemon driver.
func ReceiveEeveeGift(m *emu.Emu, romData []byte, policy MovePolicy) (CatchResult, error) {
	return receiveGiftPokemonAt(m, romData, policy, giftPokemonSpec{
		Name:    "ReceiveEeveeGift",
		Map:     celadonMansionRoofHouseMap,
		X:       eeveeGiftX,
		Y:       eeveeGiftY,
		Species: eeveeGiftSpecies,
	})
}
